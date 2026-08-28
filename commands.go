package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Berapa lama sebuah perintah masih layak dijalankan setelah dibuat.
const commandTTL = 60 * time.Second

type command struct {
	ID           string     `json:"id"`
	DeviceID     string     `json:"device_id"`
	DesiredState string     `json:"desired_state"`
	ExpiresAt    time.Time  `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
	FetchedAt    *time.Time `json:"fetched_at"`
	AckedAt      *time.Time `json:"acked_at"`
}

var (
	errStateTidakDikenal = errors.New("desired_state harus ON atau OFF")
	errTidakAdaPerintah  = errors.New("tidak ada perintah menunggu")
)

// createCommand membuat perintah baru, dan membatalkan perintah lama yang
// belum sempat diambil. Lihat catatan "mendahului" di bawah.
func createCommand(
	ctx context.Context, db *sql.DB, deviceID, state, createdBy string,
) (*command, error) {
	if state != "ON" && state != "OFF" {
		return nil, errStateTidakDikenal
	}

	now := waktuSekarang()

	// Perintah lama yang belum di-ACK langsung dianggap kedaluwarsa.
	if _, err := db.ExecContext(ctx, `
		UPDATE relay_commands
		SET expires_at = ?
		WHERE device_id = ? AND acked_at IS NULL AND expires_at > ?`,
		now, deviceID, now); err != nil {
		return nil, fmt.Errorf("membatalkan perintah lama: %w", err)
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}

	expires := now.Add(commandTTL)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO relay_commands
			(id, device_id, desired_state, expires_at, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, deviceID, state, expires, now, createdBy); err != nil {
		return nil, fmt.Errorf("menyimpan perintah: %w", err)
	}

	return &command{
		ID:           id,
		DeviceID:     deviceID,
		DesiredState: state,
		ExpiresAt:    expires,
		CreatedAt:    now,
	}, nil
}

// pendingCommand mengambil perintah yang masih berlaku untuk sebuah perangkat.
func pendingCommand(ctx context.Context, db *sql.DB, deviceID string) (*command, error) {
	now := waktuSekarang()

	var c command
	err := db.QueryRowContext(ctx, `
		SELECT id, device_id, desired_state, expires_at, created_at, fetched_at, acked_at
		FROM relay_commands
		WHERE device_id = ? AND acked_at IS NULL AND expires_at > ?
		ORDER BY created_at DESC
		LIMIT 1`, deviceID, now,
	).Scan(&c.ID, &c.DeviceID, &c.DesiredState, &c.ExpiresAt,
		&c.CreatedAt, &c.FetchedAt, &c.AckedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, errTidakAdaPerintah
	}
	if err != nil {
		return nil, fmt.Errorf("membaca perintah: %w", err)
	}

	// Dicatat kapan pertama kali diambil. Kalau perangkat mengambilnya dua kali,
	// nilai pertama dipertahankan.
	if _, err := db.ExecContext(ctx, `
		UPDATE relay_commands SET fetched_at = ?
		WHERE id = ? AND fetched_at IS NULL`, now, c.ID); err != nil {
		return nil, fmt.Errorf("menandai pengambilan: %w", err)
	}

	return &c, nil
}

// ackCommand menandai perintah sudah dijalankan, lalu menyalin keadaannya ke
// tabel devices. Aman dipanggil berkali-kali.
func ackCommand(ctx context.Context, db *sql.DB, deviceID, commandID string) error {
	now := waktuSekarang()

	res, err := db.ExecContext(ctx, `
		UPDATE relay_commands
		SET acked_at = ?
		WHERE id = ? AND device_id = ? AND acked_at IS NULL`,
		now, commandID, deviceID)
	if err != nil {
		return fmt.Errorf("menyimpan ack: %w", err)
	}

	terpengaruh, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("membaca hasil ack: %w", err)
	}

	// Nol baris berarti sudah pernah di-ACK, atau bukan milik perangkat ini.
	// Keduanya bukan alasan mengembalikan error — tapi juga bukan alasan
	// menulis ulang keadaan, karena tidak ada yang benar-benar berubah.
	if terpengaruh != 1 {
		return nil
	}

	if _, err := db.ExecContext(ctx, `
		UPDATE devices
		SET last_known_state = (SELECT desired_state FROM relay_commands WHERE id = ?),
		    last_seen_at     = ?,
		    updated_at       = ?
		WHERE id = ?`,
		commandID, now, now, deviceID); err != nil {
		return fmt.Errorf("menyimpan keadaan perangkat: %w", err)
	}

	return nil
}