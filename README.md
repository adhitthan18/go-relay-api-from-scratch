# go-relay-api-from-scratch

A small IoT relay-control backend written in Go using only the standard library's `net/http`, with no web framework and no ORM. A user signs in to a React dashboard, claims a device by scanning the QR code on its screen, and switches its relay on or off. The device (an ESP32, or the included simulator) polls the API for commands and acknowledges each one.

I built this to learn how such a system works at each layer: sessions, password hashing, rate limiting, device enrollment, and command delivery, without hiding any of it behind a library. The production version of the same idea lives in [qr-relay-platform](https://github.com/adhitthan18/qr-relay-platform).

## Highlights

- **QR provisioning.** A new device registers with a bootstrap key, then shows a short-lived pairing token (`RELAY1.<token>`, valid 5 minutes) as a QR code. Scanning it from the dashboard binds the device to the account. Only a SHA-256 hash of the token is stored.
- **Command delivery with expiry.** `ON`/`OFF` commands expire after 60 seconds. A new command supersedes one that hasn't been fetched yet, and the device must ACK each command it applies.
- **Authentication done carefully:**
  - Passwords are hashed with argon2id.
  - Session tokens are random, stored only as SHA-256 hashes, and sent in an `HttpOnly`, `SameSite=Strict`, `Secure`-by-default cookie.
  - Comparisons run in constant time.
  - A dummy hash equalizes login timing for unknown emails.
- **Rate limiting.** Logins are limited per IP (5 attempts / 15 minutes). `X-Forwarded-For` is trusted only from configured proxy CIDRs.
- **Audit log** of sign-ins (successful and failed), claims, releases, and relay commands.
- **Operational basics:**
  - Embedded SQL migrations.
  - `/health` checks the database.
  - Graceful shutdown on SIGTERM, with a non-zero exit when the listener fails, so `systemd` restarts it.
  - Server timeouts are set explicitly.
- **Single binary.** The React build is embedded with `go:embed` and served from the same origin as the API.

## Architecture

```text
 Browser (React + Vite)                  ESP32 / simulator
        │  cookie session                        │  X-Bootstrap-Key, then X-Device-Token
        ▼                                        ▼
 ┌──────────────────────── Go net/http ───────────────────────┐
 │ /login /logout /me          /api/device/register           │
 │ /devices /devices/claim     /api/device/pairing-token      │
 │ /devices/{id}/command       /api/device/claim-status       │
 │ /devices/{id}/release       /api/commands/pending, /ack    │
 │ /audit   /health /version   GET / → embedded React build   │
 └────────────────────────────────┬───────────────────────────┘
                                  ▼
                            MySQL / MariaDB
```

| Path | Contents |
|---|---|
| `*.go` | API: handlers, auth, sessions, provisioning, commands, audit, rate limiter |
| `migrations/` | 9 SQL migrations, applied at startup |
| `ui/` | React 19 + TypeScript dashboard (login, device list, QR scanner) |
| `simulator/` | A command-line device: registers, prints its pairing QR in the terminal, polls and ACKs commands |
| `deploy/` | Build script, `systemd` unit, nginx site, install script |

## Running locally

Requirements: Go 1.27+, Node 22+, and MySQL 8 or MariaDB.

```bash
# 1. Configuration (DB_*, BOOTSTRAP_KEY, ADMIN_EMAIL/ADMIN_PASSWORD for the first account)
cp deploy/env.contoh .env
#    then set PORT=8080 (the example uses 8090) and COOKIE_SECURE=false for plain http://localhost

# 2. Dashboard
cd ui && npm ci && npm run build && cd ..

# 3. API (runs migrations, then listens on $PORT, default 8080)
go run .

# 4. A simulated device in another terminal
go run ./simulator -url http://localhost:8080 -bootstrap-key "<BOOTSTRAP_KEY>"
```

Sign in, choose **Pasangkan perangkat baru** ("pair a new device"), and scan or paste the QR shown by the simulator. For UI development with hot reload, run `npm run dev` in `ui/`; Vite proxies the API paths to port 8080.

The UI text and code comments are in Indonesian.

## Tests

```bash
go vet ./...
go test ./...
```

The tests cover password hashing, the rate limiter, provisioning, and device/command behaviour. Database tests use `DB_NAME_TEST` (default `<DB_NAME>_test`) and are skipped when no test database is reachable.

## Deployment

`deploy/build.sh` builds the UI and a static `linux/amd64` binary into one archive. `deploy/pasang.sh` installs it under `/opt` with a `systemd` unit and an nginx reverse proxy on port 80. TLS is expected to terminate in front of nginx (for example at Cloudflare), and `TRUSTED_PROXIES` must then include that proxy's addresses.
