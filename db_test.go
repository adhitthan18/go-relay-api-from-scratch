package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

var testDB *sql.DB

// TestMain berjalan sekali untuk seluruh paket: menyiapkan database uji,
// menjalankan semua tes, lalu menutupnya.
func TestMain(m *testing.M) {
	_ = loadEnvFile(".env")

	db, err := bukaDBUji()
	if err != nil {
		// Mesin lain mungkin tidak punya MariaDB. Tes yang membutuhkannya akan
		// melewatkan dirinya sendiri lewat butuhDB(t), jadi `go test ./...`
		// tetap hijau — bukan merah karena alasan yang bukan salah kodenya.
		fmt.Println("database uji tidak tersedia:", err)
		os.Exit(m.Run())
	}

	testDB = db
	kode := m.Run()
	db.Close()
	os.Exit(kode)
}

func bukaDBUji() (*sql.DB, error) {
	nama := os.Getenv("DB_NAME_TEST")
	if nama == "" {
		nama = os.Getenv("DB_NAME") + "_test"
	}
	if nama == "_test" {
		return nil, fmt.Errorf("DB_NAME belum diisi")
	}

	my := mysql.NewConfig()
	my.User = os.Getenv("DB_USER")
	my.Passwd = os.Getenv("DB_PASSWORD")
	my.Net = "tcp"
	my.Addr = os.Getenv("DB_HOST") + ":" + os.Getenv("DB_PORT")
	my.DBName = nama
	my.ParseTime = true
	my.Loc = nil
	my.AllowNativePasswords = true

	db, err := sql.Open("mysql", my.FormatDSN())
	if err != nil {
		return nil, err
	}

	ctx, batal := context.WithTimeout(context.Background(), 3*time.Second)
	defer batal()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	// Skema uji dibangun dari migrasi yang sama dengan produksi. Kalau ada
	// migrasi yang rusak, tes gagal di sini — sebelum menyentuh apa pun.
	if err := runMigrations(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// butuhDB melewatkan tes bila tidak ada database, lalu mengosongkan tabelnya
// supaya setiap tes mulai dari keadaan yang sama persis.
func butuhDB(t *testing.T) *sql.DB {
	t.Helper()

	if testDB == nil {
		t.Skip("database uji tidak tersedia")
	}
	kosongkan(t, testDB)

	return testDB
}

func kosongkan(t *testing.T, db *sql.DB) {
	t.Helper()

	// DELETE, bukan TRUNCATE: TRUNCATE ditolak pada tabel yang jadi sasaran
	// foreign key. Urutannya anak dulu, baru induknya.
	for _, tabel := range []string{
		"audit_log", "relay_commands", "user_sessions", "devices", "app_users",
	} {
		if _, err := db.Exec("DELETE FROM " + tabel); err != nil {
			t.Fatalf("mengosongkan %s: %v", tabel, err)
		}
	}
}