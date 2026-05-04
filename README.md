# BEON BIN API

Self-hosted BIN (Bank Identification Number) lookup API built with Go + PostgreSQL.  
Zero subscription cost — data sourced from open-source dataset ([venelinkochev/bin-list-data](https://github.com/venelinkochev/bin-list-data)) with automatic fallback enrichment via [binlist.net](https://binlist.net).

---

## Features

- BIN lookup by 6-8 digit prefix
- Full card validation (Luhn algorithm)
- 374,000+ BIN records (offline, no external dependency)
- Auto-enrichment fallback to binlist.net for unknown BINs
- In-memory cache (TTL 30 minutes)
- API Key authentication for admin endpoints
- Docker-ready for PostgreSQL, Go binary runs natively

---

## API Endpoints

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| GET | `/api/v1/health` | - | Health check |
| GET | `/api/v1/stats` | - | Total BINs in database |
| GET | `/api/v1/bin/:number` | - | Lookup BIN info |
| GET | `/api/v1/bin/:number/validate?card=<full_card>` | - | Validate card + Luhn check |

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
    "prepaid": false,
    "bank": { "name": "Conotoxia Sp. Z O.O", "url": "", "phone": "" },
    "country": { "name": "Poland", "code": "PL", "currency": "PLN", "latitude": 52, "longitude": 20 }
  }
}
```

---

## Requirements

- Go 1.22+
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
# API tersedia di http://localhost:8080
```

---

## Instalasi di aaPanel (Production)

### Prasyarat

- aaPanel sudah terinstall di VPS
- **Docker** sudah diinstall di aaPanel (App Store > Docker)
- **Go 1.22+** sudah terinstall

Cek Go:
```bash
go version
```

Jika belum ada:
```bash
wget https://go.dev/dl/go1.22.3.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.22.3.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

---

### Step 1 — Install PostgreSQL via aaPanel Docker

Di aaPanel, masuk ke **Docker > One-Click Install**, cari **PostgreSQL** lalu klik Install.

Config yang direkomendasikan:
| Field | Value |
|-------|-------|
| Version | `16.x` |
| Port | `35432` (atau port lain yang tidak bentrok) |
| User | `postgres` |
| Password | *(catat password yang di-generate)* |

Klik **Confirm**.

---

### Step 2 — Buat Database `bindb`

Setelah PostgreSQL container jalan, cari Container ID-nya:
```bash
docker ps | grep postgres
# Contoh output: 4ce9c61a0da5   postgres:16.3 ...
```

Buat database `bindb`:
```bash
docker exec -it <CONTAINER_ID> psql -U postgres -c "CREATE DATABASE bindb;"
```

Jalankan migration (buat tabel):
```bash
docker exec -i <CONTAINER_ID> psql -U postgres -d bindb < /www/wwwroot/beon-bin-api/migrations/001_create_bins.sql
```

Verifikasi tabel terbuat:
```bash
docker exec -it <CONTAINER_ID> psql -U postgres -d bindb -c "\dt"
# Harusnya muncul: public | bins | table | postgres
```

---

### Step 3 — Clone Project

```bash
cd /www/wwwroot
git clone git@github.com:afuzapratama/beon-bin-api.git
cd beon-bin-api
```

---

### Step 4 — Konfigurasi `.env`

```bash
cp .env.example .env
nano .env
```

Sesuaikan dengan settingan PostgreSQL yang diinstall tadi:

```env
DATABASE_URL=postgres://postgres:PASSWORD_ANDA@localhost:35432/bindb?sslmode=disable
PORT=8080
ENRICHMENT_ENABLED=true
ADMIN_API_KEY=ganti-dengan-api-key-rahasia
```

> **Catatan:** Ganti `PASSWORD_ANDA` dengan password yang di-generate saat install PostgreSQL, dan `35432` dengan port yang Anda set.

---

### Step 5 — Import Dataset BIN

```bash
cd /www/wwwroot/beon-bin-api
bash scripts/import_csv.sh
```

Proses ini akan:
1. Download CSV dari `venelinkochev/bin-list-data` (~26MB)
2. Import ~374.788 records ke database

Output sukses:
```
==> Downloading BIN dataset...
==> Download complete: /www/wwwroot/beon-bin-api/data/bin-list-data.csv
==> Starting import...
Done! Imported 374788 records (0 skipped)
==> Import finished.
```

---

### Step 6 — Build Binary

```bash
cd /www/wwwroot/beon-bin-api
go build -o beon-bin-api ./cmd/api
```

Test jalankan manual:
```bash
./beon-bin-api
# Output: BIN API listening on :8080
```

---

### Step 7 — Setup Go Project di aaPanel

Di aaPanel, masuk ke **App Store > Go Project > Add Project**:

| Field | Value |
|-------|-------|
| Executable File | `/www/wwwroot/beon-bin-api/beon-bin-api` |
| Project Name | `beon-bin-api` |
| Project Port | `8080` |
| Execution Command | `beon-bin-api` |
| Environment Variables | Pilih **Load from file** → `/www/wwwroot/beon-bin-api/.env` |
| Run User | `root` |
| Startup | Centang (auto-start) |

Klik **Confirm**.

---

### Step 8 — Setup Nginx Reverse Proxy di aaPanel

Di aaPanel > **Website > Add Site**:
- Domain: `bin-api.yourdomain.com`
- PHP: `Pure Static`

Masuk ke **Site Settings > Config**, tambahkan di dalam block `server {}`:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080\;
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

Data BIN jarang berubah. Update manual tiap 3 bulan:

```bash
cd /www/wwwroot/beon-bin-api
bash scripts/import_csv.sh
```

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/bindb?sslmode=disable` | PostgreSQL connection string |
| `PORT` | `8080` | Port server |
| `ENRICHMENT_ENABLED` | `true` | Auto-enrich BIN tidak dikenal via binlist.net |
| `ADMIN_API_KEY` | - | API Key untuk endpoint admin (wajib diisi) |

---

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.22 |
| Framework | Gin |
| Database | PostgreSQL 16 |
| Driver | sqlx + lib/pq |
| Cache | In-memory (sync.Map, TTL 30m) |
| Dataset | venelinkochev/bin-list-data |
| Fallback API | binlist.net |

---

## License

MIT
