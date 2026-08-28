#!/usr/bin/env bash
#
# Dijalankan DI SERVER, dari folder hasil ekstrak paket.

set -euo pipefail

TUJUAN=/opt/belajar-relay
ENVFILE=/etc/belajar-relay.env

biru() { printf "\n\033[1;34m==>\033[0m %s\n" "$1"; }
mati() { printf "\033[1;31m✗\033[0m %s\n" "$1" >&2; exit 1; }

[[ $EUID -eq 0 ]] || mati "jalankan sebagai root"
[[ -f relay ]]    || mati "berkas 'relay' tidak ada di folder ini"

biru "Menyiapkan pengguna sistem"
# Pengguna tanpa shell dan tanpa home: kalau prosesnya dibobol, tidak ada
# tempat berpijak.
id relay >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin relay

biru "Memasang binary"
install -d -o root -g root -m 0755 "$TUJUAN"
install -o root -g root -m 0755 relay "$TUJUAN/relay"

biru "Menyiapkan environment"
if [[ ! -f "$ENVFILE" ]]; then
  install -o root -g relay -m 0640 env.contoh "$ENVFILE"
  echo "Dibuat dari contoh: $ENVFILE"
  echo "ISI DULU nilainya sebelum menjalankan service."
else
  echo "Sudah ada, tidak diubah: $ENVFILE"
fi

biru "Memasang service"
install -o root -g root -m 0644 relay.service /etc/systemd/system/relay.service
systemctl daemon-reload

biru "Selesai"
cat <<'PESAN'
Langkah berikutnya:

  1. nano /etc/belajar-relay.env      # isi password, admin, bootstrap key
  2. systemctl enable --now relay
  3. systemctl status relay
  4. journalctl -u relay -f           # lihat migrasi berjalan

Nginx:

  cp nginx-relay.conf /etc/nginx/sites-available/belajar-relay
  # ganti GANTI_HOSTNAME dengan nama domain Anda
  ln -s /etc/nginx/sites-available/belajar-relay /etc/nginx/sites-enabled/
  nginx -t && systemctl reload nginx
PESAN