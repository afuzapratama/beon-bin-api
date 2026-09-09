# BEON BIN API

Self-hosted BIN (Bank Identification Number) lookup API built with Go + PostgreSQL.  
Zero subscription cost — data sourced from open-source dataset ([venelinkochev/bin-list-data](https://github.com/venelinkochev/bin-list-data)) with optional, persistent enrichment via HandyAPI and [binlist.net](https://binlist.net).

---

## Features

- BIN lookup by exactly 6 or 8 ASCII digits, with exact-8 then 6-digit fallback
- BIN format validation without accepting or logging full card numbers
- 374,000+ BIN records (offline, no external dependency)
- Optional fallback chain: HandyAPI first, then binlist.net
- Valid enrichment results are persisted with provider provenance
- PostgreSQL-backed HandyAPI monthly quota guard across restarts/instances
- Bounded in-memory cache with positive and negative TTL
- Global and per-client rate limiting
- Versioned, checksummed database migrations at API/importer startup
- Graceful shutdown, bounded HTTP timeouts, and configurable PostgreSQL pool
- JSON access logs with request ID and PAN/query redaction
- Separate liveness/readiness probes and Prometheus metrics
- Docker-ready for PostgreSQL, Go binary runs natively

---

## API Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/api/v1/live` | - | Process liveness; tidak mengecek dependency |
| GET | `/api/v1/ready` | - | Readiness; `503` jika PostgreSQL tidak tersedia |
| GET | `/api/v1/health` | - | Alias backward-compatible untuk `/live` |
| GET | `/api/v1/stats` | - | Total BINs in database |
| GET | `/api/v1/bin/:number` | - | Lookup BIN info |
| GET | `/metrics` | - | Prometheus metrics; batasi dari internet publik |

> For security, this API only accepts an exact 6- or 8-digit BIN/IIN. Luhn checks for a
> full card number must be performed on the client so the PAN never enters
> application, proxy, or access logs. Passing Luhn only confirms checksum
> consistency; it does not prove that a card exists, is active, or can transact.

### Example Response

```json
// GET /api/v1/bin/411111
{
  "success": true,
  "powered_by": "BEON API",
  "data": {
    "bin": "411111",
    "brand": "visa",
    "type": "debit",
    "category": "classic",
    "prepaid": null,
    "bank": { "name": "Conotoxia Sp. Z O.O", "url": "", "phone": "" },
    "country": { "name": "Poland", "code": "PL", "currency": "", "latitude": null, "longitude": null }
  }
}
```

---

## Requirements

- Go 1.26.8+
- Docker & Docker Compose (untuk PostgreSQL)
- Git

---

## Instalasi Manual (Lokal / Tanpa aaPanel)

```bash
# 1. Clone repository
git clone git@github.com:afuzapratama/beon-bin-api.git
cd beon-bin-api

# 2. Copy dan edit konfigurasi
cp .env.example .env

# 3. Jalankan PostgreSQL via Docker
docker compose up -d postgres

# 4. Download dan import dataset BIN (~374k records)
bash scripts/import_csv.sh

# 5. Jalankan API
go run ./cmd/api
# API tersedia di http://localhost:8828
```

---

## Instalasi di aaPanel (Production)

### Prasyarat

- aaPanel sudah terinstall di VPS
- **Docker** sudah diinstall di aaPanel (App Store > Docker)
- Go Project tersedia dan server dapat mengakses GitHub/go.dev

---

Model deployment yang dipakai:

```text
Internet -> aaPanel Nginx/SSL -> 127.0.0.1:8828 (binary Go)
                              -> 127.0.0.1:5434 (PostgreSQL Compose)
```

PostgreSQL dibind loopback-only. Port aplikasi `8828` tidak dirilis melalui
aaPanel/firewall; trafik publik hanya masuk melalui Nginx pada port 80/443.

### Step 1 — Clone Project

```bash
cd /www/wwwroot
git clone https://github.com/afuzapratama/beon-bin-api.git
cd beon-bin-api
git pull --ff-only origin main
```

Untuk repository private, gunakan SSH deploy key dan jangan menaruh token pada URL.

---

### Step 2 — Install Go

Gunakan **Go Project > SDK Manage > All version**, install Go `1.26.8`, lalu
jadikan versi tersebut sebagai command-line version. Buka ulang terminal dan cek:

```bash
go version
```

Output harus menunjukkan `go1.26.8` atau patch yang lebih baru dalam seri yang
kompatibel.

---

### Step 3 — Konfigurasi `.env`

```bash
cp .env.example .env
openssl rand -hex 32
nano .env
```

Salin hasil `openssl` sebagai password PostgreSQL dan jangan mengirimkannya ke
chat/log. Gunakan password hex agar aman dimasukkan ke PostgreSQL URL:

```env
DATABASE_URL=postgres://postgres:PASSWORD_HEX_YANG_SAMA@127.0.0.1:5434/bindb?sslmode=disable
PORT=8828

POSTGRES_DB=bindb
POSTGRES_USER=postgres
POSTGRES_PASSWORD=PASSWORD_HEX_YANG_SAMA
POSTGRES_PORT=5434

ENRICHMENT_ENABLED=true
HANDY_API_KEY=KEY_PRIVATE_HANDY
HANDY_API_MONTHLY_LIMIT=3000
CORS_ORIGINS=https://app.domain-anda.com
```

Jika API hanya dipanggil server-to-server, biarkan `CORS_ORIGINS=` kosong. Setelah
disimpan, batasi akses file konfigurasi untuk process aaPanel:

```bash
chown root:www .env
chmod 640 .env
```

---

### Step 4 — Jalankan PostgreSQL

Pastikan Docker/Compose sudah terpasang dari aaPanel App Store, lalu:

```bash
cd /www/wwwroot/beon-bin-api
docker compose up -d postgres
docker compose ps
docker compose exec postgres pg_isready -U postgres -d bindb
```

Compose membuat database `bindb`, user, volume persisten, dan binding
`127.0.0.1:5434` secara otomatis. Jangan jalankan service `api` dari Compose
karena process API akan dikelola oleh Go Project aaPanel.

---

### Step 5 — Import Dataset BIN

```bash
cd /www/wwwroot/beon-bin-api
bash scripts/import_csv.sh
```

Proses ini akan:
1. Resolve commit upstream terbaru lalu download CSV dari commit immutable tersebut
2. Validasi header, seluruh kolom, dan semua BIN sebelum publish
3. Import atomik melalui staging table; kegagalan me-rollback seluruh refresh
4. Hapus hanya row lokal yang sudah tidak ada di dataset baru dan simpan audit versi/checksum

Output sukses:
```
==> Downloading BIN dataset at commit 023a4f68... ...
==> Starting import...
Done! Imported 374788 records, deleted 0 stale local records
Version: 023a4f68ab1b5d7edd45f03acb132f88a49db96a
SHA-256: 0583860988c7b15d2921025ee8afe89eaf4947fe31acde8ffccfc82691d13e35
==> Import finished.
```

---

### Step 6 — Build Binary

```bash
cd /www/wwwroot/beon-bin-api
mkdir -p bin
go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api
chown root:www bin/api
chmod 750 bin/api
```

Test sebagai user runtime dari root project:

```bash
sudo -u www ./bin/api
```

Pada terminal lain, pastikan `curl http://127.0.0.1:8828/api/v1/ready`
menghasilkan status `200`, lalu hentikan test dengan `Ctrl+C`.

---

### Step 7 — Setup Go Project di aaPanel

Di aaPanel, masuk ke **App Store > Go Project > Add Project**:

| Field | Value |
|-------|-------|
| Executable File | `/www/wwwroot/beon-bin-api/bin/api` |
| Project Name | `beon-bin-api` |
| Project Port | `8828` |
| Release port | Jangan dicentang |
| Execution Command | `/www/wwwroot/beon-bin-api/bin/api` |
| Environment Variables | Pilih **Load from file** → `/www/wwwroot/beon-bin-api/.env` |
| Run User | `www` |
| Startup | Centang (auto-start) |
| Domain name | Domain API; boleh dikosongkan sampai DNS siap |

Klik **Confirm**.

Pastikan menu **Security** aaPanel dan security group provider tidak membuka
port `8828` atau `5434` ke internet. Port yang perlu dibuka untuk API hanya
`80/tcp` dan `443/tcp`.

---

### Step 8 — Setup Nginx Reverse Proxy di aaPanel

Di aaPanel > **Website > Add Site**:
- Domain: `bin-api.yourdomain.com`
- PHP: `Pure Static`

Masuk ke **Site Settings > Config**, tambahkan di dalam block `server {}`:

```nginx
location / {
    proxy_pass http://127.0.0.1:8828;
    # Hindari PAN/query sensitif masuk access log proxy. Gunakan custom
    # log_format berbasis $uri jika request logging tetap diperlukan.
    access_log off;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_connect_timeout 30s;
    proxy_read_timeout 30s;
}
```

Aktifkan **SSL** via Site Settings > SSL > Let's Encrypt.

---

### Step 9 — Verifikasi

```bash
# Health check
curl https://bin-api.yourdomain.com/api/v1/health
# {"status":"ok"}

# Cek total data
curl https://bin-api.yourdomain.com/api/v1/stats
# {"powered_by":"BEON API","success":true,"total_bins":374788}

# Lookup BIN
curl https://bin-api.yourdomain.com/api/v1/bin/411111
```

---

## Update Dataset BIN

Periksa dan refresh manual secara berkala, misalnya tiap bulan:

```bash
cd /www/wwwroot/beon-bin-api
bash scripts/import_csv.sh
```

Lihat histori import dan provenance:

```sql
SELECT source, source_version, checksum_sha256, status,
       rows_read, rows_imported, rows_deleted, completed_at
FROM dataset_imports
ORDER BY completed_at DESC;
```

### Batasan sumber data

Dataset lokal adalah data komunitas berlisensi CC BY 4.0 dan terakhir diperbarui
11 Februari 2025. Dataset saat ini hanya memuat prefix 6 digit, sehingga request
8 digit akan memakai record 6 digit kecuali ada enrichment exact-8. Binlist.net
juga menyatakan datanya tidak sempurna dan free tier dibatasi 5 request/jam.
Karena itu hasil API adalah metadata best-effort, bukan dasar tunggal untuk
otorisasi pembayaran, fraud decision, atau bukti kartu aktif. Penggunaan finansial
production memerlukan sumber berlisensi/current dan verifikasi dari payment processor.

---

## Testing dan Quality Gate

Unit test dan race detector:

```bash
go test -race ./...
```

Repository integration test memakai schema PostgreSQL sementara dan tidak mengubah
schema/data utama. Arahkan hanya ke database development/test:

```bash
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5434/bindb?sslmode=disable' \
  go test -race ./internal/database ./internal/repository/postgres
```

Coverage setiap paket kritis wajib minimal 80%:

```bash
TEST_DATABASE_URL="$DATABASE_URL" bash scripts/check_coverage.sh
```

Workflow [.github/workflows/ci.yml](.github/workflows/ci.yml) menjalankan format
check, module consistency, Actionlint, Vet, Staticcheck, race/integration tests,
coverage gate, build, dan Govulncheck pada setiap push atau pull request.

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | wajib | PostgreSQL connection string; aplikasi gagal start jika kosong |
| `PORT` | `8080` | Port server; contoh lokal menggunakan `8828` |
| `ENRICHMENT_ENABLED` | `false` | Aktifkan enrichment sinkron hanya jika memahami limit upstream |
| `HANDY_API_KEY` | kosong | API key private; jika kosong, Handy dilewati dan fallback langsung ke Binlist |
| `HANDY_API_MONTHLY_LIMIT` | `3000` | Batas bersama per bulan yang disimpan atomik di PostgreSQL |
| `CORS_ORIGINS` | kosong | Daftar origin dipisahkan koma; CORS nonaktif jika kosong |
| `RATE_LIMIT_RPS` | `5` | Sustained request per detik per client |
| `RATE_LIMIT_BURST` | `10` | Burst request per client |
| `GLOBAL_RATE_LIMIT_RPS` | `100` | Sustained request per detik seluruh instance |
| `GLOBAL_RATE_LIMIT_BURST` | `200` | Burst request seluruh instance |
| `RATE_LIMIT_MAX_CLIENTS` | `10000` | Batas client limiter yang disimpan di memory |
| `DB_MAX_OPEN_CONNS` | `20` | Maksimum koneksi PostgreSQL terbuka |
| `DB_MAX_IDLE_CONNS` | `10` | Maksimum koneksi PostgreSQL idle; tidak boleh melebihi max open |
| `DB_CONN_MAX_LIFETIME` | `30m` | Umur maksimum sebuah koneksi database |
| `DB_CONN_MAX_IDLE_TIME` | `5m` | Durasi maksimum koneksi tetap idle |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | Batas waktu membaca request header |
| `HTTP_READ_TIMEOUT` | `10s` | Batas waktu membaca seluruh request |
| `HTTP_WRITE_TIMEOUT` | `30s` | Batas waktu menulis response, termasuk fallback provider |
| `HTTP_IDLE_TIMEOUT` | `60s` | Timeout koneksi keep-alive idle |
| `HTTP_SHUTDOWN_TIMEOUT` | `10s` | Deadline graceful shutdown sebelum force-close |
| `HTTP_MAX_HEADER_BYTES` | `1048576` | Ukuran maksimum request header dalam byte |
| `ADMIN_API_KEY` | - | Reserved; endpoint admin belum tersedia |

---

## Runtime dan Observability

Setiap response membawa `X-Request-ID`. Request ID aman dari client dipertahankan;
nilai yang tidak valid diganti otomatis. Access log menggunakan JSON terstruktur,
tidak mencatat query string, dan meredaksi rangkaian angka 12 digit atau lebih.

Gunakan probe berikut pada orchestrator/load balancer:

```text
Liveness:  GET /api/v1/live
Readiness: GET /api/v1/ready
```

`/live` tetap `200` selama process hidup. `/ready` melakukan `PingContext` ke
PostgreSQL dan mengembalikan `503` saat database gagal atau aplikasi sedang shutdown.

Prometheus dapat melakukan scrape pada `GET /metrics`. Metric aplikasi utama:

- `beon_bin_api_http_requests_total`
- `beon_bin_api_http_request_duration_seconds`
- `beon_bin_api_ready`

Label HTTP memakai route template seperti `/api/v1/bin/:number`, bukan nilai BIN,
agar cardinality tetap terbatas. Contoh alert untuk service down, not-ready, error
rate, dan latency tersedia di
[`deploy/prometheus/alerts.yml`](deploy/prometheus/alerts.yml). Receiver/notifikasi
Alertmanager tetap harus dikonfigurasi sesuai deployment.

Kontrak hasil lookup eksternal:

- `404`: PostgreSQL dan provider terakhir sama-sama tidak menemukan data.
- `429`: provider enrichment terakhir sedang rate-limited; response membawa `Retry-After`.
- `503`: provider/persistence mengalami kegagalan operasional.

---

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.26.8+ |
| Framework | Gin |
| Database | PostgreSQL 16 |
| Driver | sqlx + lib/pq |
| Cache | Bounded in-memory map + mutex (positive TTL 30m, negative TTL 5m) |
| Dataset | venelinkochev/bin-list-data |
| Fallback API | HandyAPI → binlist.net |

### Alur enrichment private

```text
PostgreSQL lookup
    ↓ miss
HandyAPI lookup → validasi → simpan sebagai source=handy_api
    ↓ not found / quota / 429 / error
binlist.net lookup → validasi → simpan sebagai source=binlist_net
    ↓ not found / rate limit / gagal
404 / 429 / 503
```

Quota Handy dihitung sebelum outbound request melalui tabel `enrichment_usage`,
sehingga restart aplikasi dan beberapa instance API memakai counter bulanan yang
sama. `HANDY_API_MONTHLY_LIMIT=3000` memakai batas bawah free plan sebagai guard.
Gunakan API key khusus untuk service ini; request HandyAPI dari aplikasi lain tidak
tercatat oleh guard lokal sehingga dapat membuat hitungan berbeda dari quota akun.
Pemilik deployment bertanggung jawab memastikan penggunaan dan penyimpanan hasil
sesuai izin/ketentuan akun HandyAPI yang digunakan.

Cek pemakaian quota bulan berjalan:

```sql
SELECT provider, period_start, request_count, updated_at
FROM enrichment_usage
WHERE provider = 'handy_api'
ORDER BY period_start DESC;
```

---

## License

MIT
