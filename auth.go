package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const sessionCookieName = "session"

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type meResponse struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, pesan string) {
	writeJSON(w, code, map[string]string{"error": pesan})
}

func (app *application) sessionCookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   app.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	}
}

func (app *application) login(w http.ResponseWriter, r *http.Request) {
	key := app.clientIP(r)
	if !app.loginLimiter.allow(key) {
		w.Header().Set("Retry-After", "900")
		writeError(w, http.StatusTooManyRequests, "terlalu banyak percobaan, coba lagi nanti")
		return
	}

	var req loginRequest
	body := http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "isi permintaan tidak terbaca")
		return
	}

	u, err := findUserByEmail(r.Context(), app.db, req.Email)
	if err != nil {
		// Tetap jalankan verifikasi terhadap hash palsu supaya waktu tempuhnya
		// mirip dengan jalur "password salah". Tanpa ini, selisih waktu bisa
		// dipakai menebak email mana yang terdaftar.
		_ = verifyPassword(req.Password, app.dummyHash)
		tulisAudit(app.db, catatan{Aksi: aksiLoginGagal, Detail: req.Email, IP: key})
		writeError(w, http.StatusUnauthorized, "email atau password salah")
		return
	}

	if err := verifyPassword(req.Password, u.PasswordHash); err != nil {
		tulisAudit(app.db, catatan{Aksi: aksiLoginGagal, UserID: u.ID, Detail: req.Email, IP: key})
		writeError(w, http.StatusUnauthorized, "email atau password salah")
		return
	}

	if !u.IsActive {
		writeError(w, http.StatusForbidden, "akun dinonaktifkan")
		return
	}

	token, expires, err := createSession(r.Context(), app.db, u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tidak bisa membuat session")
		return
	}

	app.loginLimiter.reset(key)
	tulisAudit(app.db, catatan{Aksi: aksiLogin, UserID: u.ID, IP: key})
	http.SetCookie(w, app.sessionCookie(token, expires))
	writeJSON(w, http.StatusOK, meResponse{Email: u.Email, Role: u.Role})
}

func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = deleteSession(r.Context(), app.db, c.Value)
	}

	hapus := app.sessionCookie("", time.Unix(0, 0))
	hapus.MaxAge = -1
	http.SetCookie(w, hapus)

	writeJSON(w, http.StatusOK, map[string]string{"status": "keluar"})
}

// requireUser membungkus handler agar hanya bisa diakses yang sudah masuk.
func (app *application) requireUser(
	next func(http.ResponseWriter, *http.Request, *user),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "belum masuk")
			return
		}

		u, err := userFromSession(r.Context(), app.db, c.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "session tidak berlaku")
			return
		}

		next(w, r, u)
	}
}

func (app *application) me(w http.ResponseWriter, r *http.Request, u *user) {
	writeJSON(w, http.StatusOK, meResponse{Email: u.Email, Role: u.Role})
}

// clientIP mengambil alamat pemanggil. Di belakang Nginx ini akan berisi
// 127.0.0.1 untuk semua orang — lihat catatan di bawah.
// clientIP menentukan alamat pemanggil, dan ini lebih berduri daripada
// kelihatannya.
//
// X-Forwarded-For bisa ditulis siapa saja. Kalau dipercaya begitu saja,
// penyerang cukup mengirim header palsu yang berbeda tiap percobaan untuk
// melewati rate limiter sepenuhnya — jauh lebih buruk daripada bug yang
// sedang kita perbaiki. Jadi header itu hanya dibaca bila permintaannya
// memang datang dari proxy yang kita kenal.
func (app *application) clientIP(r *http.Request) string {
	langsung := alamatSaja(r.RemoteAddr)

	if !app.proxyTepercaya(langsung) {
		return langsung
	}

	// Rantainya tumbuh dari kiri: klien lebih dulu, lalu tiap proxy menambahkan
	// alamat yang dilihatnya.
	//
	//   X-Forwarded-For: <klien>, <proxy1>, <proxy2>
	//
	// Ditelusuri dari kanan sambil melewati proxy yang kita percaya. Yang
	// pertama tidak dikenal adalah alamat terjauh yang masih bisa
	// dipertanggungjawabkan — di kirinya siapa pun bisa mengarang.
	bagian := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(bagian) - 1; i >= 0; i-- {
		kandidat := strings.TrimSpace(bagian[i])
		if kandidat == "" {
			continue
		}
		if !app.proxyTepercaya(kandidat) {
			return kandidat
		}
	}

	return langsung
}

func (app *application) proxyTepercaya(ip string) bool {
	alamat := net.ParseIP(ip)
	if alamat == nil {
		return false
	}

	for _, jaringan := range app.trustedProxies {
		if jaringan.Contains(alamat) {
			return true
		}
	}

	return false
}

func alamatSaja(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func (app *application) listDevices(w http.ResponseWriter, r *http.Request, u *user) {
	daftar, err := listDevices(r.Context(), app.db)
	if err != nil {
		// Pesan ke pengguna tetap umum — ia tidak perlu tahu isi query kita.
		// Tapi penyebab aslinya dicatat di log server, kalau tidak kesalahan
		// seperti jumlah kolom Scan yang meleset hanya tampak sebagai
		// "tidak bisa membaca perangkat": benar, tapi tidak menolong siapa pun.
		fmt.Println("listDevices gagal:", err)
		writeError(w, http.StatusInternalServerError, "tidak bisa membaca perangkat")
		return
	}

	writeJSON(w, http.StatusOK, daftar)
}

type createCommandRequest struct {
	State string `json:"state"`
}

func (app *application) createCommand(w http.ResponseWriter, r *http.Request, u *user) {
	deviceID := r.PathValue("id")

	var req createCommandRequest
	body := http.MaxBytesReader(w, r.Body, 1024)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "isi permintaan tidak terbaca")
		return
	}

	c, err := createCommand(r.Context(), app.db, deviceID, req.State, u.ID)
	if err != nil {
		if errors.Is(err, errStateTidakDikenal) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "tidak bisa membuat perintah")
		return
	}
	tulisAudit(app.db, catatan{
		Aksi:     aksiPerintahRelay,
		UserID:   u.ID,
		DeviceID: deviceID,
		Detail:   req.State,
		IP:       app.clientIP(r),
	})

	writeJSON(w, http.StatusCreated, c)
}

type claimDeviceRequest struct {
	QRPayload   string `json:"qr_payload"`
	DisplayName string `json:"display_name"`
}

func (app *application) claimDevice(w http.ResponseWriter, r *http.Request, u *user) {
	var req claimDeviceRequest
	body := http.MaxBytesReader(w, r.Body, 2048)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "isi permintaan tidak terbaca")
		return
	}

	deviceID, err := claimDevice(r.Context(), app.db, req.QRPayload, req.DisplayName, u.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "QR tidak berlaku atau sudah kedaluwarsa")
		return
	}

	tulisAudit(app.db, catatan{
		Aksi:     aksiPerangkatClaim,
		UserID:   u.ID,
		DeviceID: deviceID,
		Detail:   req.DisplayName,
		IP:       app.clientIP(r),
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "claimed"})
}

func (app *application) releaseDevice(w http.ResponseWriter, r *http.Request, u *user) {
	deviceID := r.PathValue("id")

	if err := releaseDevice(r.Context(), app.db, deviceID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tulisAudit(app.db, catatan{
		Aksi:     aksiPerangkatLepas,
		UserID:   u.ID,
		DeviceID: deviceID,
		IP:       app.clientIP(r),
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "pending"})
}
func (app *application) listAudit(w http.ResponseWriter, r *http.Request, u *user) {
	daftar, err := listAudit(r.Context(), app.db, 50)
	if err != nil {
		fmt.Println("listAudit gagal:", err)
		writeError(w, http.StatusInternalServerError, "tidak bisa membaca audit")
		return
	}

	writeJSON(w, http.StatusOK, daftar)
}
