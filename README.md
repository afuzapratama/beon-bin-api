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
- Docker-ready, single `docker compose up` deployment

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
- Docker & Docker Compose (for PostgreSQL)
- Git

---

## Instalasi Manual (Tanpa aaPanel)

```bash
# 1. Clone repository
git clone git@github.com:afuzapratama/beon-bin-api.git
cd beon-bin-api

# 2. Copy dan edit konfigurasi
cp .env.example .env
# Edit ADMIN_API_KEY sesuai kebutuhan

# 3. Jalankan PostgreSQL via Docker
docker compose up -d postgres

# 4. Download dan import dataset BIN (~374k records)
bash scripts/import_csv.sh

# 5. Jalankan API
go run ./cmd/api

# API tersedia di http://localhost:8080
```

---

## Instalasi di aaPanel (Recommended Production)

### Prasyarat di VPS/aaPanel

1. Masuk ke **aaPanel > App Store**, install:
   - **Docker** (aktifkan Docker Manager)
   - **PostgreSQL** (atau gunakan Docker Postgres di bawah)

2. Pastikan **Go** sudah terinstall:
   ```bash
   # Cek versi
   go version

   # Jika belum ada, install via:
   wget https://go.dev/dl/go1.22.3.linux-amd64.tar.gz
   tar -C /usr/local -xzf go1.22.3.linux-amd64.tar.gz
   echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
   source ~/.bashrc
   ```

---

### Step 1 — Upload / Clone Project

Di **aaPanel > File Manager** atau via SSH:

```bash
cd /www/wwwroot
git clone git@github.com:afuzapratama/beon-bin-api.git
cd beon-bin-api
```

---

### Step 2 — Konfigurasi Environment

```bash
cp .env.example .env
nano .env
```

Isi `.env`:

```env
DATABASE_URL=postgres://postgres:YOURPASSWORD@localhost:5432/bindb?sslmode=disable
PORT=8080
ENRICHMENT_ENABLED=true
ADMIN_API_KEY=ganti-dengan-api-key-rahasia
```

---

### Step 3 — Jalankan PostgreSQL via Docker

```bash
docker compose up -d postgres
```

Verifikasi:
```bash
docker compose ps
# Output: beon-bin-api-postgres-1   Up   0.0.0.0:5432->5432/tcp
```

---

### Step 4 — Import Dataset BIN

```bash
bash scripts/import_csv.sh
# Proses: download CSV (~26MB) + import ke DB
# Output: Done! Imported 374788 records (0 skipped)
```

---

### Step 5 — Build & Jalankan API

```bash
# Build binary
go build -o beon-bin-api ./cmd/api

# Test manual
./beon-bin-api
# Output: BIN API listening on :8080
```

---

### Step 6 — Setup Supervisor di aaPanel (Auto-restart)

Di aaPanel, masuk ke **App Store > Supervisor > Add Daemon**:

| Field | Value |
|-------|-------|
| Name | `beon-bin-api` |
| Run User | `www` atau `root` |
| Run Dir | `/www/wwwroot/beon-bin-api` |
| Command | `/www/wwwroot/beon-bin-api/beon-bin-api` |
| Processes | `1` |
| Auto Start | `Yes` |

Atau via SSH dengan systemd:

```bash
# Buat service file
cat > /etc/systemd/system/beon-bin-api.service << EOF
[Unit]
Description=BEON BIN API
After=network.target docker.service

[Service]
Type=simple
User=root
WorkingDirectory=/www/wwwroot/beon-bin-api
EnvironmentFile=/www/wwwroot/beon-bin-api/.env
ExecStart=/www/wwwroot/beon-bin-api/beon-bin-api
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable beon-bin-api
systemctl start beon-bin-api
systemctl status beon-bin-api
```

---

### Step 7 — Setup Nginx Reverse Proxy di aaPanel

Di aaPanel > **Website > Add Site**:
- Domain: `bin-api.yourdomain.com`
- PHP: `Pure Static` atau `No PHP`

Lalu masuk ke **Site Settings > Config** dan tambahkan di dalam block `server {}`:

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

Aktifkan **SSL** via aaPanel > Site Settings > SSL > Let's Encrypt.

---

### Step 8 — Verifikasi

```bash
# Health check
curl https://bin-api.yourdomain.com/api/v1/health
# {"status":"ok"}

# Stats
curl https://bin-api.yourdomain.com/api/v1/stats
# {"powered_by":"BEON API","success":true,"total_bins":374788}

# Lookup BIN
curl https://bin-api.yourdomain.com/api/v1/bin/411111
```

---

## Update Dataset BIN

Data BIN jarang berubah. Untuk update manual (tiap 3 bulan):

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
