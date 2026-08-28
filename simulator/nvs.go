package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type nvs struct {
	path         string
	HardwareID   string `json:"hardware_id"`
	BootstrapKey string `json:"bootstrap_key"`
	Secret       string `json:"secret"`
	DeviceToken  string `json:"device_token"`
}

func bukaNVS(path string) (*nvs, error) {
	n := &nvs{path: path}

	isi, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return n, nil // pertama kali menyala — kosong itu wajar
	}
	if err != nil {
		return nil, fmt.Errorf("membaca %s: %w", path, err)
	}
	if err := json.Unmarshal(isi, n); err != nil {
		return nil, fmt.Errorf("isi %s rusak: %w", path, err)
	}

	return n, nil
}

func (n *nvs) simpan() error {
	isi, err := json.MarshalIndent(n, "", "  ")
	if err != nil {
		return err
	}

	// 0600: berkas ini memuat secret dan token perangkat.
	if err := os.WriteFile(n.path, isi, 0o600); err != nil {
		return fmt.Errorf("menulis %s: %w", n.path, err)
	}

	return nil
}