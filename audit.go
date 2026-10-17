package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Aksi ditulis sebagai konstanta supaya salah ketik tertangkap compiler.
// Tanpa ini, satu "perintah_relai" yang salah eja akan jadi baris yang tidak
// pernah bisa ditemukan lagi saat dicari.
const (
	aksiLogin          = "login"
	aksiLoginGagal     = "login_gagal"
	aksiPerintahRelay  = "perintah_relay"
	aksiPerangkatClaim = "perangkat_claim"
	aksiPerangkatLepas = "perangkat_lepas"
)

type catatan struct {
	Aksi     string
	UserID   string
	DeviceID string
	Detail   string
	IP       string
}

// tulisAudit menyimpan satu baris jejak.
//
// Sengaja best-effort: kalau penulisannya gagal, tindakan yang sudah terjadi
// tidak dibatalkan. Relay yang sudah menyala tidak bisa dimundurkan hanya
// karena catatannya gagal disimpan, dan menolak perintah gara-gara log
// bermasalah akan mematikan sistem kendali justru saat paling dibutuhkan.
//
// Untuk sistem yang lebih ketat, penulisan ini masuk ke transaksi yang sama
// dengan tindakannya: keduanya berhasil, atau keduanya batal.
func tulisAudit(db *sql.DB, c catatan) {
	// Sengaja TIDAK memakai context permintaan. Kalau pengguna menutup
	// browsernya tepat setelah menekan tombol, context itu dibatalkan — padahal
	// tindakannya sudah terjadi. Jejaknya harus tetap tertulis.
	ctx, batal := context.WithTimeout(context.Background(), 5*time.Second)
	defer batal()

	id, err := newID()
	if err != nil {
		fmt.Println("audit: gagal membuat id:", err)
		return
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO audit_log (id, terjadi_at, aksi, user_id, device_id, detail, ip)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, waktuSekarang(), c.Aksi,
		kosongJadiNull(c.UserID), kosongJadiNull(c.DeviceID),
		kosongJadiNull(c.Detail), kosongJadiNull(c.IP))

	if err != nil {
		fmt.Println("audit: gagal menulis:", err)
	}
}

// kosongJadiNull membedakan "tidak berlaku" dari "diisi kosong".
// NULL di kolom device_id berarti aksi ini memang bukan tentang perangkat.
func kosongJadiNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type barisAudit struct {
	ID         string    `json:"id"`
	TerjadiAt  time.Time `json:"terjadi_at"`
	Aksi       string    `json:"aksi"`
	UserEmail  *string   `json:"user_email"`
	DeviceName *string   `json:"device_name"`
	Detail     *string   `json:"detail"`
	IP         *string   `json:"ip"`
}

func listAudit(ctx context.Context, db *sql.DB, batas int) ([]barisAudit, error) {
	if batas <= 0 || batas > 200 {
		batas = 50
	}

	// LEFT JOIN, bukan JOIN. Perangkat atau pengguna yang sudah dihapus tidak
	// boleh membuat barisnya hilang dari hasil — justru baris itu yang penting.
	rows, err := db.QueryContext(ctx, `
		SELECT a.id, a.terjadi_at, a.aksi, u.email, d.display_name, a.detail, a.ip
		  FROM audit_log a
		  LEFT JOIN app_users u ON u.id = a.user_id
		  LEFT JOIN devices   d ON d.id = a.device_id
		 ORDER BY a.terjadi_at DESC, a.id DESC
		 LIMIT ?`, batas)
	if err != nil {
		return nil, fmt.Errorf("membaca audit: %w", err)
	}
	defer rows.Close()

	daftar := []barisAudit{}
	for rows.Next() {
		var b barisAudit
		if err := rows.Scan(&b.ID, &b.TerjadiAt, &b.Aksi,
			&b.UserEmail, &b.DeviceName, &b.Detail, &b.IP); err != nil {
			return nil, fmt.Errorf("membaca baris audit: %w", err)
		}
		daftar = append(daftar, b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterasi audit: %w", err)
	}

	return daftar, nil
}
