package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type user struct {
	ID           string
	Email        string
	PasswordHash string
	Role         string
	IsActive     bool
}

var errUserNotFound = errors.New("pengguna tidak ditemukan")

func findUserByEmail(ctx context.Context, db *sql.DB, email string) (*user, error) {
	var u user
	err := db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, role, is_active
		FROM app_users
		WHERE email = ?`, strings.ToLower(email),
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.IsActive)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, errUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mencari pengguna: %w", err)
	}
	return &u, nil
}

func createUser(ctx context.Context, db *sql.DB, email, password, role string) (*user, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	_, err = db.ExecContext(ctx, `
		INSERT INTO app_users (id, email, password_hash, role, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?)`,
		id, strings.ToLower(email), hash, role, now, now)
	if err != nil {
		return nil, fmt.Errorf("menyimpan pengguna: %w", err)
	}

	return &user{ID: id, Email: strings.ToLower(email), PasswordHash: hash, Role: role, IsActive: true}, nil
}

func ensureAdmin(ctx context.Context, db *sql.DB, email, password string) error {
	if email == "" || password == "" {
		return nil
	}

	_, err := findUserByEmail(ctx, db, email)
	if err == nil {
		fmt.Println("admin sudah ada:", strings.ToLower(email))
		return nil
	}
	if !errors.Is(err, errUserNotFound) {
		return err
	}

	if _, err := createUser(ctx, db, email, password, "admin"); err != nil {
		return err
	}
	fmt.Println("admin dibuat:", strings.ToLower(email))
	return nil
}