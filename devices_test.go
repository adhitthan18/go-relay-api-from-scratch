package main

import (
	"context"
	"testing"
	"time"
)

// Tes ini menjaga celah yang tidak dilihat compiler: jumlah kolom di SELECT
// harus cocok dengan jumlah tujuan di Scan, dan urutannya harus sama. Keduanya
// hanya teks di dalam string — tidak ada yang memeriksanya selain tes ini.
func TestListDevicesMemetakanKolom(t *testing.T) {
	db := butuhDB(t)
	ctx := context.Background()

	if err := registerDevice(ctx, db, hwUji, hashToken("rahasia"), "v1"); err != nil {
		t.Fatal(err)
	}

	now := waktuSekarang()
	if _, err := db.Exec(`
		UPDATE devices
		   SET display_name = 'Relay Pompa', status = 'active',
		       last_known_state = 'ON', last_seen_at = ?, enrolled_at = ?
		 WHERE hardware_id = ?`, now, now, hwUji); err != nil {
		t.Fatal(err)
	}

	daftar, err := listDevices(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(daftar) != 1 {
		t.Fatalf("dapat %d perangkat, seharusnya 1", len(daftar))
	}

	d := daftar[0]
	if d.HardwareID != hwUji {
		t.Errorf("HardwareID = %q, mau %q", d.HardwareID, hwUji)
	}
	if d.DisplayName != "Relay Pompa" {
		t.Errorf("DisplayName = %q", d.DisplayName)
	}
	if d.Status != "active" {
		t.Errorf("Status = %q, mau \"active\"", d.Status)
	}
	if d.LastKnownState == nil || *d.LastKnownState != "ON" {
		t.Error("LastKnownState tidak terbaca sebagai \"ON\"")
	}
	if !d.Enrolled {
		t.Error("Enrolled seharusnya true")
	}
	if !d.Online {
		t.Error("Online seharusnya true — baru saja terlihat")
	}
}

func TestPerangkatLamaMenghilangJadiOffline(t *testing.T) {
	db := butuhDB(t)
	ctx := context.Background()

	if err := registerDevice(ctx, db, hwUji, hashToken("rahasia"), "v1"); err != nil {
		t.Fatal(err)
	}

	// Terakhir menyapa jauh sebelum ambang offline.
	lampau := waktuSekarang().Add(-2 * deviceOfflineAfter)
	if _, err := db.Exec(`
		UPDATE devices SET last_known_state = 'ON', last_seen_at = ? WHERE hardware_id = ?`,
		lampau, hwUji); err != nil {
		t.Fatal(err)
	}

	daftar, err := listDevices(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	d := daftar[0]
	if d.Online {
		t.Error("perangkat yang lama tidak menyapa seharusnya offline")
	}
	// Yang hilang perangkatnya, bukan relaynya. Keadaan terakhir tetap dilaporkan.
	if d.LastKnownState == nil || *d.LastKnownState != "ON" {
		t.Error("keadaan relay ikut hilang saat perangkat offline — seharusnya tetap ON")
	}
}

func TestTandaiTerlihatDihemat(t *testing.T) {
	db := butuhDB(t)
	ctx := context.Background()

	if err := registerDevice(ctx, db, hwUji, hashToken("rahasia"), "v1"); err != nil {
		t.Fatal(err)
	}

	var id string
	if err := db.QueryRow("SELECT id FROM devices WHERE hardware_id = ?", hwUji).Scan(&id); err != nil {
		t.Fatal(err)
	}

	if err := tandaiTerlihat(ctx, db, id); err != nil {
		t.Fatal(err)
	}
	var pertama time.Time
	if err := db.QueryRow("SELECT last_seen_at FROM devices WHERE id = ?", id).Scan(&pertama); err != nil {
		t.Fatal(err)
	}

	// Sapaan kedua yang datang segera sesudahnya tidak boleh menulis lagi —
	// itulah gunanya seenThrottle.
	if err := tandaiTerlihat(ctx, db, id); err != nil {
		t.Fatal(err)
	}
	var kedua time.Time
	if err := db.QueryRow("SELECT last_seen_at FROM devices WHERE id = ?", id).Scan(&kedua); err != nil {
		t.Fatal(err)
	}

	if !kedua.Equal(pertama) {
		t.Error("last_seen_at ditulis dua kali berturut-turut — throttle tidak bekerja")
	}
}