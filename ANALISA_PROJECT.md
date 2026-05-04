# Analisa Project: BIN Card API (Self-Hosted, Zero Cost)

> Tanggal analisa: 4 Mei 2026  
> Stack utama: **Go (Golang)** + **PostgreSQL / SQLite**  
> Tujuan: Memiliki BIN API sendiri tanpa biaya langganan bulanan

---

## 1. Pengertian BIN / IIN

**BIN** (Bank Identification Number) atau **IIN** (Issuer Identification Number) adalah **6–8 digit pertama** dari nomor kartu (kredit/debit/prepaid).

```
4111 1111 1111 1111
^^^^
6 digit BIN → 411111
```

Dari BIN kita bisa tahu:
| Info | Contoh |
|---|---|
| Brand / Jaringan | Visa, Mastercard, Amex, JCB, UnionPay |
| Tipe kartu | Credit, Debit, Prepaid |
| Bank penerbit | BCA, Mandiri, BNI, CIMB, dll |
| Negara asal | ID (Indonesia), US, SG, dll |
| Tingkatan | Classic, Gold, Platinum, Business |

---

## 2. Masalah & Kenapa Harus Bayar?

Sebagian besar layanan BIN API berbayar karena:
- Data BIN dikompilasi secara manual dari berbagai sumber
- Perlu update rutin ketika bank mengeluarkan BIN baru
- Contoh layanan berbayar: `bincodes.com`, `bincheck.io`, `rapidapi BIN lookup` → $5–$50/bulan

---

## 3. Solusi Gratis: Dari Mana Kita Dapat Data BIN?

Ada **4 sumber gratis** yang bisa digabungkan:

### Sumber A: Dataset Open-Source di GitHub (Prioritas Utama)

Repository-repository ini berisi data BIN dalam format CSV/JSON:

| Repo | Isi | Format | Keterangan |
|---|---|---|---|
| `venelinkochev/bin-list-data` | ~500,000+ BIN entries | CSV | **Aktif di-maintain, update terakhir Feb 2025** |
| `storm-buster/creditcard-BIN-lists` | JSON per brand | JSON | Mudah di-import |
| `nicehash/binlist` | JSON format clean | JSON | Format sudah terstruktur bagus |

> **Cara download:**
> ```bash
> git clone https://github.com/venelinkochev/bin-list-data
> # lalu ambil file bin-list-data.csv
> ```

### Sumber B: BinList.net Free API (Rate Limited)

- URL: `https://lookup.binlist.net/{BIN}`
- **Gratis**, limit 10 request/menit tanpa API key
- Bisa dipakai untuk **enrichment** (isi data yang kosong dari dataset lokal)
- Response berupa JSON lengkap: brand, type, bank, country

```bash
curl https://lookup.binlist.net/45717360
# Response:
# {"number":{},"scheme":"visa","type":"debit","brand":"Visa/Dankort","country":{"numeric":"208","alpha2":"DK",...},"bank":{...}}
```

### Sumber C: Wikipedia IIN Ranges (Supplementary)

- Wikipedia punya artikel lengkap tentang IIN ranges
- URL: https://en.wikipedia.org/wiki/Payment_card_number
- Bisa dipakai untuk validasi prefix brand (prefix 4 = Visa, 5 = Mastercard, dll)

### Sumber D: Self-Populate dari Transaksi (Long Term)

- Setiap kali ada request BIN baru yang belum ada di DB
- Otomatis hit ke binlist.net untuk enrichment
- Simpan hasilnya ke local DB → semakin lama semakin lengkap
- Ini strategi **lazy population**

---

## 4. Arsitektur Sistem

```
┌─────────────────────────────────────────────────┐
│                  CLIENT / APP                    │
└──────────────────────┬──────────────────────────┘
                       │ HTTP Request
                       ▼
┌─────────────────────────────────────────────────┐
│              GO REST API (Gin/Fiber)             │
│                                                 │
│  GET /bin/:number → Handler                     │
│  GET /bins/batch  → Handler                     │
│  POST /bins/import → Admin Handler              │
│                                                 │
│    ┌─────────────┐    ┌──────────────────────┐  │
│    │   Cache      │    │  BIN Lookup Service  │  │
│    │  (Redis/     │←──│                      │  │
│    │   In-memory) │    │  1. Cek local DB     │  │
│    └─────────────┘    │  2. Cek Cache        │  │
│                       │  3. Fallback: remote  │  │
│                       └──────────┬───────────┘  │
└──────────────────────────────────┼──────────────┘
                                   │
               ┌───────────────────┼────────────────┐
               ▼                   ▼                 ▼
    ┌────────────────┐  ┌───────────────┐  ┌──────────────────┐
    │  PostgreSQL/   │  │  Redis Cache  │  │ binlist.net API  │
    │  SQLite DB     │  │  (Optional)   │  │ (Fallback/Free)  │
    │  (Primary)     │  │               │  │                  │
    └────────────────┘  └───────────────┘  └──────────────────┘
```

---

## 5. Database Schema

```sql
CREATE TABLE bins (
    id          BIGSERIAL PRIMARY KEY,
    bin         VARCHAR(8)   NOT NULL UNIQUE,   -- 6 atau 8 digit
    brand       VARCHAR(50),                    -- visa, mastercard, amex, jcb
    type        VARCHAR(20),                    -- credit, debit, prepaid
    category    VARCHAR(50),                    -- classic, gold, platinum, business
    bank_name   VARCHAR(200),                   -- Nama bank penerbit
    bank_url    VARCHAR(200),                   -- Website bank
    bank_phone  VARCHAR(100),                   -- Telepon bank
    country_name VARCHAR(100),                  -- Nama negara
    country_code VARCHAR(3),                    -- ISO Alpha-2: ID, US, SG
    country_currency VARCHAR(10),               -- IDR, USD, SGD
    country_latitude  DECIMAL(9,6),
    country_longitude DECIMAL(9,6),
    prepaid     BOOLEAN DEFAULT FALSE,
    source      VARCHAR(50) DEFAULT 'local',    -- local, binlist_net
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_bins_bin ON bins(bin);
CREATE INDEX idx_bins_brand ON bins(brand);
CREATE INDEX idx_bins_country ON bins(country_code);
```

---

## 6. API Endpoints yang Akan Dibuat

| Method | Endpoint | Deskripsi |
|---|---|---|
| `GET` | `/api/v1/bin/:number` | Lookup satu BIN |
| `GET` | `/api/v1/bin/:number/validate` | Validasi format kartu |
| `POST` | `/api/v1/bins/batch` | Lookup banyak BIN sekaligus |
| `GET` | `/api/v1/health` | Health check |
| `POST` | `/api/v1/admin/import` | Import CSV dataset (auth required) |
| `GET` | `/api/v1/stats` | Statistik DB (total BINs, dll) |

### Contoh Response

```json
// GET /api/v1/bin/411111
{
  "success": true,
  "data": {
    "bin": "411111",
    "brand": "visa",
    "type": "credit",
    "category": "classic",
    "prepaid": false,
    "bank": {
      "name": "JPMorgan Chase Bank N.A.",
      "url": "https://www.chase.com",
      "phone": "+1-800-432-3117"
    },
    "country": {
      "name": "United States",
      "code": "US",
      "currency": "USD",
      "latitude": 37.09,
      "longitude": -95.71
    }
  }
}

// Jika tidak ditemukan
{
  "success": false,
  "error": "BIN not found",
  "code": 404
}
```

---

## 7. Struktur Folder Project (Go)

```
beon-bin-api/
├── cmd/
│   ├── api/
│   │   └── main.go              # Entry point server
│   └── importer/
│       └── main.go              # CLI tool import CSV
├── internal/
│   ├── handler/
│   │   ├── bin.go               # Handler endpoint BIN
│   │   └── admin.go             # Handler admin (import)
│   ├── service/
│   │   ├── bin_service.go       # Business logic lookup
│   │   └── enrichment.go        # Auto-enrich dari remote
│   ├── repository/
│   │   ├── bin_repo.go          # Interface repo
│   │   └── postgres/
│   │       └── bin_repo.go      # Implementasi PostgreSQL
│   ├── model/
│   │   └── bin.go               # Struct model BIN
│   └── middleware/
│       ├── auth.go              # API key auth
│       ├── ratelimit.go         # Rate limiting
│       └── cors.go              # CORS config
├── pkg/
│   ├── binlist/
│   │   └── client.go            # HTTP client ke binlist.net
│   ├── cache/
│   │   └── cache.go             # Wrapper cache (memory/redis)
│   └── validator/
│       └── card.go              # Validasi Luhn algorithm
├── migrations/
│   └── 001_create_bins.sql      # SQL migration
├── data/
│   └── .gitkeep                 # Taruh CSV dataset di sini
├── scripts/
│   └── import_csv.sh            # Script helper import
├── docker-compose.yml
├── Dockerfile
├── go.mod
├── go.sum
├── .env.example
└── README.md
```

---

## 8. Tahapan Pengerjaan (Roadmap)

> Legend: ✅ Selesai · 🔄 Sedang berjalan · ⬜ Belum dikerjakan

### Phase 1: Setup & Data ✅ Scaffold selesai — ⬜ Data belum diimport
- [x] `go mod init github.com/beon/bin-api`
- [x] Scaffold struktur folder project Go
- [x] Buat migration SQL schema (`migrations/001_create_bins.sql`)
- [x] Buat CLI importer (`cmd/importer/main.go`) — mapping kolom CSV venelinkochev sudah benar
- [x] Buat `docker-compose.yml` dengan service PostgreSQL
- [x] Buat `.env.example`
- [x] Buat `scripts/import_csv.sh` (download + import otomatis)
- [ ] **⬜ Jalankan `docker compose up -d postgres`**
- [ ] **⬜ Jalankan `./scripts/import_csv.sh` (download + import ~500K BINs)**
- [ ] **⬜ Verifikasi data masuk: `GET /api/v1/stats`**

### Phase 2: Core API ✅ Kode selesai — ⬜ Belum ditest live
- [x] Setup Gin framework (`cmd/api/main.go`)
- [x] Buat layer repository PostgreSQL (`internal/repository/postgres/`)
- [x] Buat service layer dengan lookup logic (`internal/service/bin_service.go`)
- [x] Handler `GET /api/v1/bin/:number` dan `GET /api/v1/stats`
- [x] Handler `GET /api/v1/bin/:number/validate` (Luhn check)
- [x] Integrasi fallback ke binlist.net jika BIN tidak ada di DB
- [ ] **⬜ Test manual dengan curl setelah DB terisi data**

### Phase 3: Caching & Performance ✅ In-memory cache sudah ada
- [x] In-memory cache TTL 30 menit (`pkg/cache/cache.go`)
- [ ] **⬜ Benchmark test (target 1000+ req/detik)**
- [ ] **⬜ Tambahkan rate limiting per IP** (middleware belum dibuat)
- [ ] ⬜ Opsional: integrasi Redis untuk multi-instance

### Phase 4: Auth & Admin ✅ Sebagian selesai
- [x] API Key authentication via header `X-API-Key` (`internal/middleware/auth.go`)
- [x] Validasi Luhn algorithm (`pkg/validator/card.go`)
- [ ] **⬜ Endpoint admin `POST /api/v1/admin/import` untuk re-import data**
- [ ] **⬜ Batch lookup endpoint `POST /api/v1/bins/batch`**

### Phase 5: Deployment ✅ Dockerfile & Compose sudah ada
- [x] `Dockerfile` (multi-stage build)
- [x] `docker-compose.yml` (API + PostgreSQL)
- [ ] **⬜ Test `docker compose up` full stack**
- [ ] **⬜ Setup nginx reverse proxy**
- [ ] **⬜ Deploy ke VPS**
- [ ] **⬜ Setup domain + SSL**

---

### Urutan Langkah Selanjutnya (yang harus dikerjakan sekarang)

```
1. docker compose up -d postgres
2. cp .env.example .env  (edit ADMIN_API_KEY)
3. ./scripts/import_csv.sh
4. go run ./cmd/api
5. curl http://localhost:8080/api/v1/stats
6. curl http://localhost:8080/api/v1/bin/411111
```

---

## 9. Estimasi Biaya

| Item | Biaya |
|---|---|
| BIN Dataset (GitHub open-source) | **Rp 0** |
| binlist.net API fallback | **Rp 0** (rate limited) |
| Go language | **Rp 0** |
| PostgreSQL | **Rp 0** (self-hosted) |
| VPS Hetzner CX11 (2GB RAM) | ~**Rp 35.000/bulan** |
| Domain .com | ~**Rp 200.000/tahun** |
| **Total** | **~Rp 35.000/bulan** vs langganan API Rp 500K+/bulan |

---

## 10. Dependency Go yang Akan Digunakan

```
github.com/gin-gonic/gin          - HTTP framework
github.com/lib/pq                  - PostgreSQL driver
github.com/jmoiron/sqlx            - SQL helper
github.com/joho/godotenv           - .env loader
github.com/dgraph-io/ristretto     - High-performance cache
github.com/go-playground/validator - Input validation
go.uber.org/zap                    - Structured logging
github.com/spf13/cobra             - CLI tool (importer)
```

---

## 11. Cara Update Data BIN

Data BIN tidak berubah terlalu sering. Strategi update:

1. **Manual quarterly**: Tiap 3 bulan, re-download CSV dari GitHub repo dan re-import
2. **Auto-enrich**: Setiap request BIN yang tidak ada di DB → hit binlist.net → simpan hasilnya
3. **Monitor GitHub**: Watch repo `venelinkochev/bin-list-data` untuk dapat notif update

---

## 12. Keamanan API

- Semua endpoint public dilindungi rate limiting (max 60 req/menit per IP)
- Endpoint admin (`/admin/*`) wajib API Key di header
- API Key di-hash dengan bcrypt sebelum disimpan
- Log semua request dengan IP untuk monitoring abuse
- Tidak menyimpan nomor kartu penuh — hanya BIN (6–8 digit), bukan PAN

---

## 13. Langkah Pertama Sekarang

```bash
# 1. Init Go module
cd /mnt/DataDrive/Github/beon-bin-api
go mod init github.com/beon/bin-api

# 2. Download dataset BIN
wget https://raw.githubusercontent.com/venelinkochev/bin-list-data/master/bin-list-data.csv \
     -O data/bin-list-data.csv

# 3. Mulai coding struktur dasar
```

---

*Dokumen ini akan diupdate seiring pengerjaan project.*
