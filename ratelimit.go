package main

import (
	"context"
	"sync"
	"time"
)

type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string]*window
	max    int
	period time.Duration

	// Sumber waktu bisa diganti saat test, sehingga test tidak perlu menunggu.
	now func() time.Time
}

type window struct {
	count int
	until time.Time
}

func newRateLimiter(max int, period time.Duration) *rateLimiter {
	return &rateLimiter{
		hits:   make(map[string]*window),
		max:    max,
		period: period,
		now:    time.Now,
	}
}

// allow mencatat satu percobaan, dan mengembalikan false bila sudah melewati batas.
func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	sekarang := rl.now()
	w, ada := rl.hits[key]

	if !ada || sekarang.After(w.until) {
		rl.hits[key] = &window{count: 1, until: sekarang.Add(rl.period)}
		return true
	}

	if w.count >= rl.max {
		return false
	}

	w.count++
	return true
}

// reset dipanggil setelah login berhasil, agar percobaan yang sah tidak
// menghabiskan jatah pemilik akun sendiri.
func (rl *rateLimiter) reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.hits, key)
}

func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	sekarang := rl.now()
	for key, w := range rl.hits {
		if sekarang.After(w.until) {
			delete(rl.hits, key)
		}
	}
}

// jalankanPembersih membuang entri kedaluwarsa berkala sampai ctx selesai.
func (rl *rateLimiter) jalankanPembersih(ctx context.Context, setiap time.Duration) {
	t := time.NewTicker(setiap)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rl.cleanup()
		}
	}
}