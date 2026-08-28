package main

import (
	"strings"
	"testing"
)

func TestHashPasswordSelaluBerbeda(t *testing.T) {
	a, err := hashPassword("rahasia123")
	if err != nil {
		t.Fatal(err)
	}
	b, err := hashPassword("rahasia123")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("password sama seharusnya menghasilkan hash berbeda — salt-nya tidak acak")
	}
	if !strings.HasPrefix(a, "$argon2id$") {
		t.Fatalf("format hash tidak sesuai: %s", a)
	}
}

func TestVerifyPassword(t *testing.T) {
	hash, err := hashPassword("rahasia123")
	if err != nil {
		t.Fatal(err)
	}

	if err := verifyPassword("rahasia123", hash); err != nil {
		t.Fatal("password benar seharusnya diterima:", err)
	}

	if err := verifyPassword("rahasia124", hash); err == nil {
		t.Fatal("password salah seharusnya ditolak")
	}
}

func TestVerifyPasswordHashRusak(t *testing.T) {
	if err := verifyPassword("apa saja", "bukan-hash"); err == nil {
		t.Fatal("hash yang rusak seharusnya ditolak, bukan diterima")
	}
}