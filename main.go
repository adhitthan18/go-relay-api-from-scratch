package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type application struct {
	db             *sql.DB
	version        string
	cookieSecure   bool
	dummyHash      string
	loginLimiter   *rateLimiter
	bootstrapKey   string
	trustedProxies []*net.IPNet
}

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

type versionResponse struct {
	Version string `json:"version"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Println("konfigurasi:", err)
		os.Exit(1)
	}

	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		fmt.Println("DSN tidak valid:", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	startCtx, startCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startCancel()

	if err := db.PingContext(startCtx); err != nil {
		fmt.Println("tidak bisa terhubung ke database:", err)
		os.Exit(1)
	}
	fmt.Println("database terhubung")

	if err := runMigrations(startCtx, db); err != nil {
		fmt.Println("migrasi gagal:", err)
		os.Exit(1)
	}

	if err := ensureAdmin(startCtx, db, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		fmt.Println("menyiapkan admin gagal:", err)
		os.Exit(1)
	}

	// Hash palsu untuk menyamakan waktu tempuh saat email tidak ditemukan.
	dummyHash, err := hashPassword("hash-pembanding-yang-tidak-dipakai")
	if err != nil {
		fmt.Println("menyiapkan hash pembanding gagal:", err)
		os.Exit(1)
	}

	app := &application{
		db:             db,
		version:        "0.1.0",
		cookieSecure:   cfg.CookieSecure,
		dummyHash:      dummyHash,
		loginLimiter:   newRateLimiter(5, 15*time.Minute),
		bootstrapKey:   cfg.BootstrapKey,
		trustedProxies: cfg.TrustedProxies,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go app.loginLimiter.jalankanPembersih(ctx, time.Minute)
	go jalankanPembersihSession(ctx, db, time.Hour)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", app.health)
	mux.HandleFunc("GET /version", app.versionHandler)
	mux.HandleFunc("GET /slow", app.slow)
	mux.HandleFunc("POST /login", app.login)
	mux.HandleFunc("POST /logout", app.logout)
	mux.HandleFunc("GET /me", app.requireUser(app.me))
	mux.HandleFunc("GET /devices", app.requireUser(app.listDevices))
	mux.HandleFunc("POST /devices/claim", app.requireUser(app.claimDevice))
	mux.HandleFunc("POST /devices/{id}/release", app.requireUser(app.releaseDevice))
	mux.HandleFunc("POST /api/device/register", app.requireBootstrapKey(app.registerDevice))
	mux.HandleFunc("POST /api/device/pairing-token", app.requireBootstrapKey(app.pairingToken))
	mux.HandleFunc("POST /api/device/claim-status", app.requireBootstrapKey(app.claimStatus))
	mux.HandleFunc("GET /api/devices/me", app.requireDevice(app.deviceMe))
	mux.HandleFunc("POST /devices/{id}/command", app.requireUser(app.createCommand))
	mux.HandleFunc("GET /api/commands/pending", app.requireDevice(app.pendingCommand))
	mux.HandleFunc("POST /api/commands/ack", app.requireDevice(app.ackCommand))
	mux.HandleFunc("GET /audit", app.requireUser(app.listAudit))

	// Penampung sisa: apa pun yang tidak cocok dengan rute di atas dilayani
	// sebagai berkas dashboard. Sejak Go 1.22 ServeMux memilih pola yang paling
	// spesifik, jadi "GET /devices" tetap menang atas "GET /".
	web, err := webHandler()
	if err != nil {
		fmt.Println("menyiapkan dashboard gagal:", err)
		os.Exit(1)
	}
	mux.Handle("GET /", web)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Kanal berkapasitas 1 supaya goroutine bisa melapor lalu selesai, tanpa
	// menunggu ada yang membaca.
	gagalServer := make(chan error, 1)

	go func() {
		fmt.Println("server jalan di http://localhost:" + cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			gagalServer <- err
		}
	}()

	// Dua alasan berhenti, dan keduanya harus ditunggu bersamaan. Sebelumnya
	// hanya sinyal yang ditunggu — jadi server yang gagal mengikat port
	// membuat proses menganggur selamanya, dan systemd mengira ia sehat.
	select {
	case <-ctx.Done():
		fmt.Println("\nsinyal diterima, menyelesaikan permintaan yang sedang jalan...")

	case err := <-gagalServer:
		fmt.Println("server gagal:", err)
		// Keluar dengan status bukan nol: inilah yang membuat Restart=on-failure
		// di systemd benar-benar bekerja.
		os.Exit(1)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Println("terpaksa dihentikan:", err)
	}
	fmt.Println("server berhenti dengan rapi")
}

func (app *application) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	res := healthResponse{Status: "ok", Database: "ok"}
	code := http.StatusOK

	if err := app.db.PingContext(ctx); err != nil {
		res.Status = "degraded"
		res.Database = "unreachable"
		code = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(res)
}

func (app *application) versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versionResponse{Version: app.version})
}

func (app *application) slow(w http.ResponseWriter, r *http.Request) {
	time.Sleep(5 * time.Second)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(healthResponse{Status: "ok", Database: "tidak diperiksa"})
}
