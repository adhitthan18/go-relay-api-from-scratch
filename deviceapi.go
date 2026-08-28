package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const deviceTokenHeader = "X-Device-Token"

const bootstrapKeyHeader = "X-Bootstrap-Key"

func (app *application) requireBootstrapKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kunci := strings.TrimSpace(r.Header.Get(bootstrapKeyHeader))
		if subtle.ConstantTimeCompare([]byte(kunci), []byte(app.bootstrapKey)) != 1 {
			writeError(w, http.StatusUnauthorized, "bootstrap key tidak berlaku")
			return
		}
		next(w, r)
	}
}

type registerRequest struct {
	HardwareID string `json:"hardware_id"`
	SecretHash string `json:"secret_hash"`
	Firmware   string `json:"firmware"`
}

func (app *application) registerDevice(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	body := http.MaxBytesReader(w, r.Body, 2048)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "isi permintaan tidak terbaca")
		return
	}

	if err := registerDevice(r.Context(), app.db, req.HardwareID, req.SecretHash, req.Firmware); err != nil {
		writeError(w, http.StatusBadRequest, "pendaftaran ditolak")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "pending"})
}

// Dipakai dua rute berikutnya — keduanya membuktikan diri dengan cara sama.
type provRequest struct {
	HardwareID string `json:"hardware_id"`
	Secret     string `json:"secret"`
}

func (app *application) pairingToken(w http.ResponseWriter, r *http.Request) {
	var req provRequest
	body := http.MaxBytesReader(w, r.Body, 2048)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "isi permintaan tidak terbaca")
		return
	}

	payload, err := issuePairingToken(r.Context(), app.db, req.HardwareID, req.Secret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "permintaan pairing ditolak")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"qr_payload":    payload,
		"berlaku_detik": int(pairingTokenTTL.Seconds()),
	})
}

func (app *application) claimStatus(w http.ResponseWriter, r *http.Request) {
	var req provRequest
	body := http.MaxBytesReader(w, r.Body, 2048)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "isi permintaan tidak terbaca")
		return
	}

	status, token, err := claimStatus(r.Context(), app.db, req.HardwareID, req.Secret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "permintaan ditolak")
		return
	}

	jawab := map[string]string{"status": status}
	// Token hanya ikut sekali seumur hidup perangkat, tepat pada perpindahan
	// claimed -> active. Panggilan berikutnya menjawab "active" tanpa token.
	if token != "" {
		jawab["device_token"] = token
	}
	writeJSON(w, http.StatusOK, jawab)
}

// requireDevice menjaga rute yang hanya boleh diakses perangkat yang sudah enroll.
func (app *application) requireDevice(
	next func(http.ResponseWriter, *http.Request, *device),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(r.Header.Get(deviceTokenHeader))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "token perangkat tidak ada")
			return
		}

		d, err := deviceFromToken(r.Context(), app.db, token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "token perangkat tidak berlaku")
			return
		}
				// Permintaan apa pun yang lolos autentikasi membuktikan perangkat hidup —
		// termasuk polling yang tidak menemukan perintah apa-apa.
		if err := tandaiTerlihat(r.Context(), app.db, d.ID); err != nil {
			// Gagal mencatat bukan alasan menolak permintaannya.
			fmt.Println("menandai last_seen gagal:", err)
		}

		next(w, r, d)
	}
}

func (app *application) deviceMe(w http.ResponseWriter, r *http.Request, d *device) {
	writeJSON(w, http.StatusOK, d)
}

func (app *application) pendingCommand(w http.ResponseWriter, r *http.Request, d *device) {
	c, err := pendingCommand(r.Context(), app.db, d.ID)
	if err != nil {
		// Tidak ada perintah bukan kesalahan — itu keadaan normal.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type ackRequest struct {
	CommandID string `json:"command_id"`
}

func (app *application) ackCommand(w http.ResponseWriter, r *http.Request, d *device) {
	var req ackRequest
	body := http.MaxBytesReader(w, r.Body, 1024)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "isi permintaan tidak terbaca")
		return
	}

	if err := ackCommand(r.Context(), app.db, d.ID, req.CommandID); err != nil {
		writeError(w, http.StatusInternalServerError, "tidak bisa menyimpan ack")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "diterima"})
}