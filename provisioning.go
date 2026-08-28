package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	labelVersi      = "RELAY1."
	pairingTokenTTL = 5 * time.Minute
)

var errPerangkatTidakCocok = errors.New("hardware id atau secret tidak cocok")

// 16 byte sudah lebih dari cukup untuk kredensial berumur lima menit, dan
// menghasilkan 22 karakter — separuh panjang token session. QR-nya jadi jauh
// lebih lega, dan QR yang lega lebih mudah dipindai dari layar kecil.
func newPairingToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("membuat token pairing: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// registerDevice mencatat perangkat yang memperkenalkan dirinya sendiri.
//
// Idempoten: perangkat yang sama boleh mendaftar berkali-kali, dan itu memang
// terjadi tiap kali ia menyala ulang sebelum sempat di-claim.
func registerDevice(ctx context.Context, db *sql.DB, hardwareID, secretHash, firmware string) error {
	hardwareID = strings.TrimSpace(hardwareID)
	if hardwareID == "" {
		return errors.New("hardware_id wajib diisi")
	}
	if len(secretHash) != 64 {
		return errors.New("secret_hash harus 64 karakter heksadesimal")
	}

	id, err := newID()
	if err != nil {
		return err
	}
	now := waktuSekarang()

	// Satu perjalanan ke database, tanpa celah antara "periksa apakah sudah
	// ada" dan "buat kalau belum".
	//
	// Perhatikan IF(...) di baris pertama UPDATE. Perangkat yang sudah di-claim
	// TIDAK boleh mengganti secret-nya hanya dengan mendaftar ulang. Tanpa
	// penjagaan itu, siapa pun yang tahu hardware_id bisa merebut perangkat
	// orang lain cukup dengan mengirimkan secret_hash miliknya sendiri.
	_, err = db.ExecContext(ctx, `
		INSERT INTO devices
			(id, hardware_id, display_name, device_secret_hash, status, firmware, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'pending', ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			device_secret_hash = IF(status = 'pending', VALUES(device_secret_hash), device_secret_hash),
			firmware           = VALUES(firmware),
			updated_at         = VALUES(updated_at)`,
		id, hardwareID, hardwareID, secretHash, firmware, now, now)

	if err != nil {
		return fmt.Errorf("mendaftarkan perangkat: %w", err)
	}
	return nil
}

// issuePairingToken menerbitkan token sekali pakai untuk ditampilkan sebagai QR.
//
// Hanya perangkat berstatus pending yang boleh meminta. Yang sudah di-claim
// tidak butuh QR lagi, dan menerbitkannya justru membuka jalan bagi orang lain
// untuk mengklaim ulang perangkat yang sudah ada pemiliknya.
func issuePairingToken(ctx context.Context, db *sql.DB, hardwareID, secret string) (string, error) {
	token, err := newPairingToken()
	if err != nil {
		return "", err
	}
	now := waktuSekarang()

	res, err := db.ExecContext(ctx, `
		UPDATE devices
		   SET pairing_token_hash = ?,
		       pairing_expires_at = ?,
		       updated_at         = ?
		 WHERE hardware_id        = ?
		   AND device_secret_hash = ?
		   AND status             = 'pending'`,
		hashToken(token), now.Add(pairingTokenTTL), now,
		strings.TrimSpace(hardwareID), hashToken(secret))
	if err != nil {
		return "", fmt.Errorf("menerbitkan token pairing: %w", err)
	}

	terpengaruh, err := res.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("membaca hasil pairing: %w", err)
	}
	if terpengaruh != 1 {
		return "", errPerangkatTidakCocok
	}

	return labelVersi + token, nil
}

// claimStatus menjawab pertanyaan yang ditanyakan perangkat berulang kali:
// "sudah boleh jalan belum?"
//
// Token perangkat dibuat DI SINI, bukan saat admin menekan Claim. Kalau dibuat
// saat Claim, server harus menyimpannya dalam bentuk asli sampai perangkat
// datang mengambil — dan itu berarti ada kunci telanjang mengendap di database
// entah berapa lama. Dengan cara ini yang tersimpan selalu hanya sidik jarinya.
func claimStatus(ctx context.Context, db *sql.DB, hardwareID, secret string) (string, string, error) {
	hardwareID = strings.TrimSpace(hardwareID)
	secretHash := hashToken(secret)

	var status string
	var punyaToken bool
	err := db.QueryRowContext(ctx, `
		SELECT status, device_token_hash IS NOT NULL
		  FROM devices
		 WHERE hardware_id = ? AND device_secret_hash = ?`,
		hardwareID, secretHash,
	).Scan(&status, &punyaToken)

	if errors.Is(err, sql.ErrNoRows) {
		return "", "", errPerangkatTidakCocok
	}
	if err != nil {
		return "", "", fmt.Errorf("membaca status claim: %w", err)
	}

	// Belum di-claim, atau sudah pernah mengambil tokennya.
	if status != "claimed" || punyaToken {
		return status, "", nil
	}

	token, err := newSessionToken()
	if err != nil {
		return "", "", err
	}
	now := waktuSekarang()

	// Syarat di WHERE mengulang apa yang baru saja dibaca SELECT di atas. Itu
	// disengaja: di antara kedua perintah, permintaan lain bisa saja sudah
	// mengambil tokennya. Hanya satu yang akan mendapat RowsAffected == 1.
	res, err := db.ExecContext(ctx, `
		UPDATE devices
		   SET device_token_hash  = ?,
		       status             = 'active',
		       enrolled_at        = ?,
		       pairing_token_hash = NULL,
		       pairing_expires_at = NULL,
		       updated_at         = ?
		 WHERE hardware_id        = ?
		   AND device_secret_hash = ?
		   AND status             = 'claimed'
		   AND device_token_hash IS NULL`,
		hashToken(token), now, now, hardwareID, secretHash)
	if err != nil {
		return "", "", fmt.Errorf("menerbitkan token perangkat: %w", err)
	}

	terpengaruh, err := res.RowsAffected()
	if err != nil {
		return "", "", fmt.Errorf("membaca hasil penerbitan: %w", err)
	}
	if terpengaruh != 1 {
		// Kalah balapan — permintaan lain sudah mengambilnya lebih dulu.
		return "active", "", nil
	}

	return "claimed", token, nil
}
var errQRTidakBerlaku = errors.New("QR tidak dikenal atau sudah kedaluwarsa")

func claimDevice(ctx context.Context, db *sql.DB, qrPayload, displayName, userID string) (string, error) {
	token, berawalan := strings.CutPrefix(strings.TrimSpace(qrPayload), labelVersi)
	if !berawalan || token == "" {
		return "", errQRTidakBerlaku
	}

	displayName = strings.TrimSpace(displayName)
	now := waktuSekarang()
	tokenHash := hashToken(token)

	var id string
	err := db.QueryRowContext(ctx, `
		SELECT id FROM devices
		 WHERE pairing_token_hash = ? AND pairing_expires_at > ? AND status = 'pending'`,
		tokenHash, now).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		return "", errQRTidakBerlaku
	}
	if err != nil {
		return "", fmt.Errorf("mencari perangkat: %w", err)
	}

	// Syaratnya diulang di WHERE. Antara SELECT dan UPDATE, admin lain bisa
	// saja sudah mengklaim perangkat yang sama — hanya satu boleh menang.
	res, err := db.ExecContext(ctx, `
		UPDATE devices
		   SET status             = 'claimed',
		       display_name       = IF(? = '', display_name, ?),
		       claimed_at         = ?,
		       claimed_by         = ?,
		       pairing_token_hash = NULL,
		       pairing_expires_at = NULL,
		       updated_at         = ?
		 WHERE id                 = ?
		   AND pairing_token_hash = ?
		   AND status             = 'pending'`,
		displayName, displayName, now, userID, now, id, tokenHash)
	if err != nil {
		return "", fmt.Errorf("mengklaim perangkat: %w", err)
	}

	terpengaruh, err := res.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("membaca hasil claim: %w", err)
	}
	if terpengaruh != 1 {
		return "", errQRTidakBerlaku
	}

	return id, nil
}
// releaseDevice melepas perangkat dari sistem.
//
// Tokennya dicabut dan statusnya kembali ke pending. Perangkat akan menyadari
// tokennya tidak berlaku lagi, mendaftar ulang, lalu menampilkan QR baru —
// tepat perilaku yang Anda minta: QR muncul saat admin melepas perangkat.
//
// device_secret_hash sengaja TIDAK dihapus. Perangkat menyimpan secret-nya
// seumur hidup di NVS, jadi membiarkannya membuat perangkat langsung bisa
// meminta QR baru tanpa perlu mendaftar dari nol.
func releaseDevice(ctx context.Context, db *sql.DB, deviceID string) error {
	now := waktuSekarang()

	res, err := db.ExecContext(ctx, `
		UPDATE devices
		   SET status             = 'pending',
		       device_token_hash  = NULL,
		       enrolled_at        = NULL,
		       claimed_at         = NULL,
		       claimed_by         = NULL,
		       pairing_token_hash = NULL,
		       pairing_expires_at = NULL,
		       last_known_state   = NULL,
		       updated_at         = ?
		 WHERE id = ? AND status <> 'pending'`,
		now, deviceID)
	if err != nil {
		return fmt.Errorf("melepas perangkat: %w", err)
	}

	terpengaruh, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("membaca hasil release: %w", err)
	}
	if terpengaruh != 1 {
		return errors.New("perangkat tidak ditemukan atau memang belum diklaim")
	}

	return nil
}