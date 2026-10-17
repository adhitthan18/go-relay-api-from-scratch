package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	firmwareVersi  = "sim-1.0.0"
	jedaPollClaim  = 3 * time.Second
	segarkanQRTiap = 4 * time.Minute // token server berlaku 5 menit
)

type command struct {
	ID           string `json:"id"`
	DesiredState string `json:"desired_state"`
}

func main() {
	baseURL := flag.String("url", "http://localhost:8080", "alamat API")
	hardwareID := flag.String("hardware-id", "SIM-ESP32-0001", "identitas perangkat")
	bootstrapKey := flag.String("bootstrap-key", "", "kunci bootstrap — cukup diisi sekali")
	nvsPath := flag.String("nvs", "simulator-nvs.json", "berkas yang meniru NVS")
	interval := flag.Duration("interval", 2*time.Second, "jeda antar polling perintah")
	invert := flag.Bool("invert", true, "balik warna QR — untuk terminal berlatar gelap")
	reset := flag.Bool("reset", false, "lupakan secret dan token, minta QR baru")
	skipAck := flag.Bool("skip-ack", false, "sengaja tidak mengirim ACK")
	flag.Parse()

	n, err := bukaNVS(*nvsPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if n.HardwareID == "" {
		n.HardwareID = *hardwareID
	}

	// Kunci bootstrap tinggal di NVS, meniru firmware yang membawanya sejak
	// di-flash. Diberikan sekali lewat -bootstrap-key, setelah itu menetap.
	if *bootstrapKey != "" {
		n.BootstrapKey = *bootstrapKey
	}
	if n.BootstrapKey == "" {
		fmt.Println("kunci bootstrap belum ada di NVS.")
		fmt.Println("Jalankan sekali dengan -bootstrap-key <kunci dari .env>;")
		fmt.Println("nilainya tersimpan dan tidak perlu diketik lagi.")
		os.Exit(1)
	}

	// -reset melupakan identitas perangkat, TAPI mempertahankan kunci
	// bootstrap — persis seperti perangkat yang dilepas admin lalu mencari
	// pemilik baru. Firmware-nya tidak ikut hilang saat itu terjadi.
	if *reset {
		n.Secret = ""
		n.DeviceToken = ""
		fmt.Println("secret dan token dilupakan — perangkat akan meminta QR baru")
	}

	if err := n.simpan(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := &klien{
		http:    &http.Client{Timeout: 5 * time.Second},
		baseURL: *baseURL,
		key:     n.BootstrapKey,
	}

	fmt.Printf("perangkat: %s\n\n", n.HardwareID)

	if err := pastikanToken(ctx, c, n, *invert); err != nil {
		if ctx.Err() == nil {
			fmt.Println("\nprovisioning gagal:", err)
			os.Exit(1)
		}
		fmt.Println("\nsimulator berhenti")
		return
	}

	jalankanRelay(ctx, c, n, *interval, *skipAck)
}

// pastikanToken menjalankan seluruh alur provisioning sampai perangkat punya
// token perangkat. Sama seperti provEnsureApiKey di firmware asli.
func pastikanToken(ctx context.Context, c *klien, n *nvs, invert bool) error {
	if n.DeviceToken != "" {
		fmt.Println("token ditemukan di NVS — langsung jalan")
		return nil
	}

	// Secret dibuat sekali seumur hidup perangkat, lalu tidak pernah berubah.
	// 24 byte acak jadi 48 karakter heksadesimal, sama seperti firmware Anda.
	if n.Secret == "" {
		s, err := secretBaru()
		if err != nil {
			return err
		}
		n.Secret = s
		if err := n.simpan(); err != nil {
			return err
		}
		fmt.Println("secret baru dibuat dan disimpan ke NVS")
	}

	// Yang dikirim sidik jarinya, bukan secret-nya. Server tidak pernah
	// menyimpan secret asli — sama seperti password pengguna.
	if err := c.register(ctx, n.HardwareID, sha256Hex(n.Secret)); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	fmt.Println("terdaftar — menunggu admin melakukan claim")

	var kedaluwarsaQR time.Time
	tick := time.NewTicker(jedaPollClaim)
	defer tick.Stop()

	for {
		if time.Now().After(kedaluwarsaQR) {
			payload, err := c.pairingToken(ctx, n.HardwareID, n.Secret)
			if err != nil {
				return fmt.Errorf("meminta token pairing: %w", err)
			}
			gambarQR(payload, invert)
			kedaluwarsaQR = time.Now().Add(segarkanQRTiap)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}

		status, token, err := c.claimStatus(ctx, n.HardwareID, n.Secret)
		if err != nil {
			fmt.Println("  gagal memeriksa status:", err)
			continue
		}

		switch {
		case token != "":
			n.DeviceToken = token
			if err := n.simpan(); err != nil {
				return fmt.Errorf("menyimpan token: %w", err)
			}
			fmt.Print("\ntoken diterima dan tersimpan di NVS — perangkat aktif\n\n")
			return nil

		case status == "active":
			// Server menganggap perangkat ini sudah mengambil tokennya, tapi
			// kita tidak memilikinya. Token hanya diterbitkan sekali, jadi
			// tidak ada cara memintanya lagi. Ini cabang PROV_NEEDS_RESET di
			// firmware Anda — hanya admin yang bisa memulihkannya.
			return fmt.Errorf("server menganggap perangkat ini sudah aktif, tapi " +
				"tokennya tidak ada di NVS.\n  Minta admin menekan Release, atau " +
				"jalankan ulang dengan -reset")

		default:
			fmt.Printf("  status: %s\n", status)
		}
	}
}

func jalankanRelay(ctx context.Context, c *klien, n *nvs, interval time.Duration, skipAck bool) {
	relay := "OFF"
	fmt.Printf("polling tiap %s, relay awal: %s\n\n", interval, relay)

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nsimulator berhenti")
			return

		case <-t.C:
			cmd, err := c.ambilPerintah(ctx, n.DeviceToken)
			if err != nil {
				fmt.Println("  gagal menghubungi server:", err)
				continue
			}
			if cmd == nil {
				continue // 204: tidak ada perintah, keadaan normal
			}

			if cmd.DesiredState == relay {
				fmt.Printf("  perintah %-3s — relay sudah %s, tidak ada perubahan\n",
					cmd.DesiredState, relay)
			} else {
				fmt.Printf("  perintah %-3s — relay %s → %s\n",
					cmd.DesiredState, relay, cmd.DesiredState)
				relay = cmd.DesiredState
			}

			if skipAck {
				fmt.Println("  ACK sengaja tidak dikirim")
				continue
			}
			if err := c.kirimAck(ctx, n.DeviceToken, cmd.ID); err != nil {
				fmt.Println("  gagal mengirim ACK:", err)
			}
		}
	}
}

func gambarQR(payload string, invert bool) {
	qr, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		fmt.Println("gagal membuat QR:", err)
		return
	}

	fmt.Println()
	fmt.Print(qr.ToSmallString(invert))
	fmt.Println(payload)
	fmt.Println("Pindai QR di atas, atau tempel teks itu ke kolom manual di dashboard.")
	fmt.Println()
}

func secretBaru() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("membuat secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
