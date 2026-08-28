#!/usr/bin/env bash
#
# Membangun paket deploy: dashboard React di-build, lalu disisipkan ke dalam
# satu binary Linux bersama migrasi dan seluruh API.
#
# Dijalankan dari mesin mana pun yang punya Go dan Node — hasilnya sama, karena
# -trimpath membuang jejak path lokal dari binary.

set -euo pipefail

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$SRC"

OUT="$SRC/belajar-relay.tar.gz"
KERJA="$(mktemp -d)"
trap 'rm -rf "$KERJA"' EXIT

biru()  { printf "\n\033[1;34m==>\033[0m %s\n" "$1"; }
mati()  { printf "\033[1;31m✗\033[0m %s\n" "$1" >&2; exit 1; }

biru "Membangun dashboard"
( cd ui && npm ci --silent && npm run build ) || mati "build dashboard gagal"

biru "Menjalankan tes"
# Tes lewat kalau database uji tidak ada, jadi ini tidak menghalangi build di
# mesin yang bersih — tapi kalau ada, kesalahan tertangkap sebelum naik.
go test ./... >/dev/null || mati "ada tes yang gagal — deploy dibatalkan"

biru "Silang-kompilasi ke Linux"
# CGO_ENABLED=0 menghasilkan binary yang benar-benar statis: tidak bergantung
# pada glibc versi tertentu, jadi ia jalan di distro Linux mana pun.
# -s -w membuang tabel simbol dan info debug, memangkas ukurannya.
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags="-s -w" -o "$KERJA/relay" . || mati "kompilasi gagal"

biru "Mengemas"
cp deploy/relay.service     "$KERJA/"
cp deploy/nginx-relay.conf  "$KERJA/"
cp deploy/env.contoh        "$KERJA/"
cp deploy/pasang.sh         "$KERJA/"
chmod +x "$KERJA/pasang.sh"

# --owner/--group=0 menghapus uid Windows dari arsip sejak awal, jadi sisi
# Linux tidak perlu tahu apa-apa tentang mesin pembuatnya.
tar --owner=0 --group=0 --numeric-owner -czf "$OUT" -C "$KERJA" .

biru "Selesai"
printf "Paket  : %s (%s)\n" "$OUT" "$(du -h "$OUT" | cut -f1)"
printf "Binary : %s\n" "$(du -h "$KERJA/relay" | cut -f1)"