package main

import (
	"crypto/rand"
	"fmt"
	"time"
)

// newID menghasilkan UUID versi 4 dari 16 byte acak.
// Ditulis sendiri agar tidak menambah dependensi untuk sesuatu sesederhana ini.
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("membuat id: %w", err)
	}

	// Empat bit penanda versi (4) dan dua bit penanda varian (RFC 4122).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
// waktuSekarang mengembalikan waktu UTC yang dipotong ke detik — persis
// sepresisi kolom DATETIME di MySQL.
//
// Dua alasan: nilai yang dikembalikan API selalu sama dengan yang benar-benar
// tersimpan, dan seluruh sistem menyimpan waktu dalam satu zona saja. Konversi
// ke waktu lokal dilakukan di ujung, saat ditampilkan ke manusia.
func waktuSekarang() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}