package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type klien struct {
	http    *http.Client
	baseURL string
	key     string
}

// postProv mengirim permintaan provisioning: selalu JSON, selalu membawa
// bootstrap key. Ditulis sekali agar tiga pemanggilnya tidak mengulang
// urusan header dan penguraian balasan.
func (c *klien) postProv(ctx context.Context, jalur string, isi, keluar any) error {
	body, err := json.Marshal(isi)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+jalur, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bootstrap-Key", c.key)

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		io.Copy(io.Discard, res.Body)
		return fmt.Errorf("status %d dari %s", res.StatusCode, jalur)
	}
	if keluar == nil {
		io.Copy(io.Discard, res.Body)
		return nil
	}

	return json.NewDecoder(res.Body).Decode(keluar)
}

func (c *klien) register(ctx context.Context, hardwareID, secretHash string) error {
	return c.postProv(ctx, "/api/device/register", map[string]string{
		"hardware_id": hardwareID,
		"secret_hash": secretHash,
		"firmware":    firmwareVersi,
	}, nil)
}

func (c *klien) pairingToken(ctx context.Context, hardwareID, secret string) (string, error) {
	var jawab struct {
		QRPayload string `json:"qr_payload"`
	}
	err := c.postProv(ctx, "/api/device/pairing-token", map[string]string{
		"hardware_id": hardwareID,
		"secret":      secret,
	}, &jawab)

	return jawab.QRPayload, err
}

// claimStatus mengembalikan status dan — hanya sekali, saat perpindahan
// claimed -> active — token perangkatnya.
func (c *klien) claimStatus(ctx context.Context, hardwareID, secret string) (string, string, error) {
	var jawab struct {
		Status      string `json:"status"`
		DeviceToken string `json:"device_token"`
	}
	err := c.postProv(ctx, "/api/device/claim-status", map[string]string{
		"hardware_id": hardwareID,
		"secret":      secret,
	}, &jawab)

	return jawab.Status, jawab.DeviceToken, err
}

func (c *klien) ambilPerintah(ctx context.Context, token string) (*command, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/commands/pending", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Device-Token", token)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusNoContent:
		return nil, nil
	case http.StatusOK:
		var cmd command
		if err := json.NewDecoder(res.Body).Decode(&cmd); err != nil {
			return nil, fmt.Errorf("membaca perintah: %w", err)
		}
		return &cmd, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("token perangkat ditolak")
	default:
		return nil, fmt.Errorf("status tidak diduga: %d", res.StatusCode)
	}
}

func (c *klien) kirimAck(ctx context.Context, token, commandID string) error {
	isi, err := json.Marshal(map[string]string{"command_id": commandID})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/commands/ack", bytes.NewReader(isi))
	if err != nil {
		return err
	}
	req.Header.Set("X-Device-Token", token)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// Habiskan isi balasan supaya koneksinya bisa dipakai ulang.
	io.Copy(io.Discard, res.Body)

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("status ack: %d", res.StatusCode)
	}
	return nil
}