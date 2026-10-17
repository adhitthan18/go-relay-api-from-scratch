package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const sessionDuration = 12 * time.Hour

var errSessionInvalid = errors.New("session tidak berlaku")

// Token acak 256 bit. Inilah yang dikirim ke browser.
func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("membuat token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Sidik jari token, inilah yang disimpan di database.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func createSession(ctx context.Context, db *sql.DB, userID string) (string, time.Time, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", time.Time{}, err
	}

	now := time.Now()
	expires := now.Add(sessionDuration)

	_, err = db.ExecContext(ctx, `
		INSERT INTO user_sessions (token_hash, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?)`,
		hashToken(token), userID, expires, now)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("menyimpan session: %w", err)
	}

	return token, expires, nil
}

func userFromSession(ctx context.Context, db *sql.DB, token string) (*user, error) {
	var u user
	err := db.QueryRowContext(ctx, `
		SELECT u.id, u.email, u.password_hash, u.role, u.is_active
		FROM user_sessions s
		JOIN app_users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`,
		hashToken(token), time.Now(),
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.IsActive)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, errSessionInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("membaca session: %w", err)
	}
	if !u.IsActive {
		return nil, errSessionInvalid
	}
	return &u, nil
}

func deleteSession(ctx context.Context, db *sql.DB, token string) error {
	if _, err := db.ExecContext(ctx,
		"DELETE FROM user_sessions WHERE token_hash = ?", hashToken(token)); err != nil {
		return fmt.Errorf("menghapus session: %w", err)
	}
	return nil
}

// hapusSessionKedaluwarsa membuang baris yang masa berlakunya sudah lewat.
//
// Session yang kedaluwarsa memang sudah ditolak saat dipakai — query di
// findSessionUser menyaringnya. Menghapusnya bukan soal keamanan, melainkan
// supaya tabelnya tidak tumbuh selamanya.
func hapusSessionKedaluwarsa(ctx context.Context, db *sql.DB) (int64, error) {
	res, err := db.ExecContext(ctx,
		"DELETE FROM user_sessions WHERE expires_at <= ?", waktuSekarang())
	if err != nil {
		return 0, fmt.Errorf("menghapus session kedaluwarsa: %w", err)
	}

	return res.RowsAffected()
}

// jalankanPembersihSession berjalan di goroutine sendiri sampai ctx selesai —
// pola yang sama dengan pembersih rate limiter.
func jalankanPembersihSession(ctx context.Context, db *sql.DB, setiap time.Duration) {
	t := time.NewTicker(setiap)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := hapusSessionKedaluwarsa(ctx, db)
			if err != nil {
				fmt.Println("pembersih session:", err)
				continue
			}
			if n > 0 {
				fmt.Printf("pembersih session: %d baris dihapus\n", n)
			}
		}
	}
}
