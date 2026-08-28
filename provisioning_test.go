package main

import (
	"context"
	"testing"
	"time"
)

const hwUji = "ESP32-UJI-0001"

func TestRegisterDeviceIdempoten(t *testing.T) {
	db := butuhDB(t)
	ctx := context.Background()

	// Perangkat yang menyala ulang sebelum di-claim akan mendaftar berkali-kali.
	for i := 1; i <= 3; i++ {
		if err := registerDevice(ctx, db, hwUji, hashToken("rahasia"), "v1"); err != nil {
			t.Fatalf("register ke-%d gagal: %v", i, err)
		}
	}

	var jumlah int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM devices WHERE hardware_id = ?", hwUji).Scan(&jumlah); err != nil {
		t.Fatal(err)
	}
	if jumlah != 1 {
		t.Fatalf("3 kali mendaftar menghasilkan %d baris, seharusnya 1", jumlah)
	}
}

// Penjagaan paling penting di seluruh alur provisioning: perangkat yang sudah
// aktif tidak boleh mengganti secret-nya hanya dengan mendaftar ulang.
func TestRegisterTidakBisaMerebutPerangkatAktif(t *testing.T) {
	db := butuhDB(t)
	ctx := context.Background()

	asli := hashToken("secret-pemilik-asli")
	if err := registerDevice(ctx, db, hwUji, asli, "v1"); err != nil {
		t.Fatal(err)
	}

	// Naikkan ke active, meniru perangkat yang sudah selesai provisioning.
	if _, err := db.Exec(
		"UPDATE devices SET status='active', device_token_hash=? WHERE hardware_id=?",
		hashToken("token-perangkat"), hwUji); err != nil {
		t.Fatal(err)
	}

	// Penyerang tahu hardware_id, lalu mendaftar dengan secret miliknya.
	if err := registerDevice(ctx, db, hwUji, hashToken("secret-penyerang"), "v1"); err != nil {
		t.Fatal(err)
	}

	var tersimpan string
	if err := db.QueryRow(
		"SELECT device_secret_hash FROM devices WHERE hardware_id=?", hwUji).Scan(&tersimpan); err != nil {
		t.Fatal(err)
	}
	if tersimpan != asli {
		t.Fatal("secret perangkat aktif berhasil ditimpa — perangkat bisa direbut orang lain")
	}
}

func TestTokenPerangkatHanyaDiterbitkanSekali(t *testing.T) {
	db := butuhDB(t)
	ctx := context.Background()
	secret := "rahasia"

	if err := registerDevice(ctx, db, hwUji, hashToken(secret), "v1"); err != nil {
		t.Fatal(err)
	}
	payload, err := issuePairingToken(ctx, db, hwUji, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claimDevice(ctx, db, payload, "Relay Uji", ""); err != nil {
		t.Fatal(err)
	}

	status, token, err := claimStatus(ctx, db, hwUji, secret)
	if err != nil {
		t.Fatal(err)
	}
	if status != "claimed" || token == "" {
		t.Fatalf("panggilan pertama seharusnya memberi token; status=%q token kosong=%v",
			status, token == "")
	}

	// Token hilang di perjalanan? Sengaja tidak ada cara memintanya lagi.
	status, token, err = claimStatus(ctx, db, hwUji, secret)
	if err != nil {
		t.Fatal(err)
	}
	if status != "active" || token != "" {
		t.Fatalf("panggilan kedua seharusnya active tanpa token; status=%q token=%q", status, token)
	}
}

func TestQRSekaliPakai(t *testing.T) {
	db := butuhDB(t)
	ctx := context.Background()
	secret := "rahasia"

	if err := registerDevice(ctx, db, hwUji, hashToken(secret), "v1"); err != nil {
		t.Fatal(err)
	}
	payload, err := issuePairingToken(ctx, db, hwUji, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claimDevice(ctx, db, payload, "Relay Uji", ""); err != nil {
		t.Fatal(err)
	}

	// Foto QR yang sama, dipakai orang lain sesudahnya.
	if _, err := claimDevice(ctx, db, payload, "Rampasan", ""); err == nil {
		t.Fatal("QR yang sudah dipakai masih bisa dipakai lagi")
	}
}

func TestQRKedaluwarsaDitolak(t *testing.T) {
	db := butuhDB(t)
	ctx := context.Background()
	secret := "rahasia"

	if err := registerDevice(ctx, db, hwUji, hashToken(secret), "v1"); err != nil {
		t.Fatal(err)
	}
	payload, err := issuePairingToken(ctx, db, hwUji, secret)
	if err != nil {
		t.Fatal(err)
	}

	// Mundurkan masa berlakunya, bukan menunggu lima menit sungguhan.
	if _, err := db.Exec(
		"UPDATE devices SET pairing_expires_at = ? WHERE hardware_id = ?",
		waktuSekarang().Add(-time.Second), hwUji); err != nil {
		t.Fatal(err)
	}

	if _, err := claimDevice(ctx, db, payload, "Terlambat", ""); err == nil {
		t.Fatal("QR yang sudah lewat waktu masih diterima")
	}
}