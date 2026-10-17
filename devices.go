package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type device struct {
	ID             string     `json:"id"`
	HardwareID     string     `json:"hardware_id"`
	DisplayName    string     `json:"display_name"`
	Status         string     `json:"status"`
	LastKnownState *string    `json:"last_known_state"`
	LastSeenAt     *time.Time `json:"last_seen_at"`
	Online         bool       `json:"online"`
	Enrolled       bool       `json:"enrolled"`
	EnrolledAt     *time.Time `json:"enrolled_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

func listDevices(ctx context.Context, db *sql.DB) ([]device, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, hardware_id, display_name, status, last_known_state, last_seen_at,
		       enrolled_at, created_at
		FROM devices
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("membaca perangkat: %w", err)
	}
	defer rows.Close()

	// Satu patokan waktu untuk seluruh daftar. Kalau dipanggil per baris, dua
	// perangkat bisa dinilai terhadap detik yang berbeda.
	now := waktuSekarang()
	daftar := []device{}

	for rows.Next() {
		var d device
		if err := rows.Scan(&d.ID, &d.HardwareID, &d.DisplayName, &d.Status,
			&d.LastKnownState, &d.LastSeenAt, &d.EnrolledAt, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("membaca baris perangkat: %w", err)
		}
		d.Enrolled = d.EnrolledAt != nil
		d.Online = d.LastSeenAt != nil && now.Sub(*d.LastSeenAt) < deviceOfflineAfter
		daftar = append(daftar, d)
	}

	// Error bisa muncul di tengah iterasi, bukan hanya saat query dijalankan.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterasi perangkat: %w", err)
	}

	return daftar, nil
}

// deviceFromToken mencari perangkat berdasarkan token yang dikirimnya.
func deviceFromToken(ctx context.Context, db *sql.DB, token string) (*device, error) {
	var d device
	err := db.QueryRowContext(ctx, `
		SELECT id, hardware_id, display_name, last_known_state, last_seen_at,
		       enrolled_at, created_at
		FROM devices
		WHERE device_token_hash = ? AND enrolled_at IS NOT NULL`,
		hashToken(token),
	).Scan(&d.ID, &d.HardwareID, &d.DisplayName,
		&d.LastKnownState, &d.LastSeenAt, &d.EnrolledAt, &d.CreatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("token perangkat tidak berlaku")
	}
	if err != nil {
		return nil, fmt.Errorf("membaca perangkat: %w", err)
	}

	d.Enrolled = true
	return &d, nil
}

// Perangkat menyapa tiap 2 detik. Menulis last_seen_at setiap kali berarti
// satu tulisan per perangkat per 2 detik — dengan 1000 perangkat itu 500
// tulisan per detik hanya untuk mencatat "saya masih hidup".
//
// Klausa terakhir membuatnya menulis hanya kalau catatan lama sudah usang.
// Ketelitiannya jadi 15 detik, dan itu cukup: ambang offline nanti 60 detik.
const seenThrottle = 15 * time.Second

// Ambang offline. Harus jauh di atas seenThrottle (15 detik) — kalau terlalu
// rapat, perangkat sehat akan berkedip antara online dan offline hanya karena
// tulisan last_seen_at sengaja dihemat.
const deviceOfflineAfter = 60 * time.Second

func tandaiTerlihat(ctx context.Context, db *sql.DB, deviceID string) error {
	now := waktuSekarang()

	_, err := db.ExecContext(ctx, `
		UPDATE devices
		SET last_seen_at = ?
		WHERE id = ? AND (last_seen_at IS NULL OR last_seen_at < ?)`,
		now, deviceID, now.Add(-seenThrottle))

	return err
}
