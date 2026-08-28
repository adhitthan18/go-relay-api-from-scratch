package main

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	waktu := time.Now()
	rl := newRateLimiter(3, time.Minute)
	rl.now = func() time.Time { return waktu }

	for i := 1; i <= 3; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("percobaan ke-%d seharusnya masih diizinkan", i)
		}
	}

	if rl.allow("1.2.3.4") {
		t.Fatal("percobaan ke-4 seharusnya ditolak")
	}

	if !rl.allow("5.6.7.8") {
		t.Fatal("alamat lain seharusnya punya jatah sendiri")
	}

	waktu = waktu.Add(time.Minute + time.Second)
	if !rl.allow("1.2.3.4") {
		t.Fatal("setelah jendela lewat seharusnya diizinkan lagi")
	}
}

func TestRateLimiterReset(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)

	rl.allow("a")
	rl.allow("a")
	if rl.allow("a") {
		t.Fatal("sudah melewati batas")
	}

	rl.reset("a")
	if !rl.allow("a") {
		t.Fatal("setelah reset seharusnya diizinkan lagi")
	}
}