# Analisa Project: BIN Card API (Self-Hosted, Zero Cost)

> Tanggal analisa awal: 4 Mei 2026
> Audit terakhir: 9 September 2026
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
| `venelinkochev/bin-list-data` | 374.788 BIN entries | CSV | Data komunitas 6 digit; update terakhir 11 Feb 2025 |
| `storm-buster/creditcard-BIN-lists` | JSON per brand | JSON | Mudah di-import |
| `nicehash/binlist` | JSON format clean | JSON | Format sudah terstruktur bagus |

> **Cara download:**
> ```bash
> git clone https://github.com/venelinkochev/bin-list-data
> # lalu ambil file bin-list-data.csv
> ```

### Sumber B: BinList.net Free API (Rate Limited)

- URL: `https://lookup.binlist.net/{BIN}`
- **Gratis**, limit resmi 5 request/jam dengan burst 5 tanpa API key
- Bisa dipakai untuk **enrichment** (isi data yang kosong dari dataset lokal)
- Response berupa JSON lengkap: brand, type, bank, country

```bash
curl https://lookup.binlist.net/45717360
# Response:
# {"number":{},"scheme":"visa","type":"debit","brand":"Visa/Dankort","country":{"numeric":"208","alpha2":"DK",...},"bank":{...}}
```

### Sumber fallback private: HandyAPI

- Dipakai sebelum Binlist hanya ketika `HANDY_API_KEY` terisi
- Guard konservatif 3.000 request/bulan tersimpan di PostgreSQL
- Respons valid disimpan sebagai `source = 'handy_api'` agar request berikutnya lokal
- Jika quota/429/error, lookup otomatis diteruskan ke Binlist

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
    │  PostgreSQL/   │  │  Redis Cache  │  │ Handy → Binlist │
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
    prepaid     BOOLEAN,
    source      VARCHAR(50) DEFAULT 'local',    -- local, binlist_net
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_bins_bin ON bins(bin);
CREATE INDEX idx_bins_brand ON bins(brand);
CREATE INDEX idx_bins_country ON bins(country_code);

-- migrations/002_dataset_integrity.sql juga menambahkan constraint BIN
-- ASCII 6/8 digit dan tabel dataset_imports untuk audit refresh.
```

---

## 6. API Endpoints

| Method | Endpoint | Deskripsi |
|---|---|---|
| `GET` | `/api/v1/bin/:number` | Lookup satu BIN |
| Client-side | Luhn check | Tidak menerima/mengirim full PAN ke API |
| Planned | `/api/v1/bins/batch` | Belum diimplementasikan |
| `GET` | `/api/v1/live` | Liveness process |
| `GET` | `/api/v1/ready` | Readiness PostgreSQL |
| `GET` | `/api/v1/health` | Alias backward-compatible untuk liveness |
| `GET` | `/metrics` | Prometheus metrics; akses perlu dibatasi saat deployment |
| Planned | `/api/v1/admin/import` | Belum diimplementasikan |
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
    "prepaid": null,
    "bank": {
      "name": "JPMorgan Chase Bank N.A.",
      "url": "https://www.chase.com",
      "phone": "+1-800-432-3117"
    },
    "country": {
      "name": "United States",
      "code": "US",
      "currency": "USD",
      "latitude": null,
      "longitude": null
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
│   ├── database/
│   │   └── migrate.go           # Versioned migration runner
│   ├── handler/
│   │   ├── bin.go               # Handler endpoint BIN
│   │   └── health.go            # Live/readiness probes
│   ├── middleware/
│   │   ├── auth.go              # API key auth
│   │   ├── ratelimit.go         # Rate limiting
│   │   ├── request_id.go        # Request correlation
│   │   └── logger.go            # JSON access log + redaction
│   ├── model/
│   │   └── bin.go               # Struct model BIN
│   ├── observability/
│   │   └── metrics.go           # Prometheus metrics
│   ├── repository/
│   │   ├── repository.go        # Interface repo
│   │   └── postgres/
│   │       └── bin_repo.go      # Implementasi PostgreSQL
│   ├── requestcontext/
│   │   └── request_id.go        # Request ID di context
│   └── service/
│       └── bin_service.go       # Lookup + enrichment chain
├── pkg/
│   ├── binlist/
│   │   └── client.go            # HTTP client ke binlist.net
│   ├── handyapi/
│   │   └── client.go            # HTTP client ke HandyAPI
│   ├── cache/
│   │   └── cache.go             # Wrapper cache (memory/redis)
│   └── validator/
│       └── card.go              # Validasi Luhn algorithm
├── migrations/
│   ├── embed.go                 # Embed SQL ke binary
│   └── 001..004_*.sql           # Ordered SQL migrations
├── deploy/prometheus/
│   └── alerts.yml               # Alerting rules
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
>
> Status di bawah telah ditinjau ulang pada 8 September 2026. Phase 1–5
> mencatat pembangunan MVP. Phase 6–10 adalah acuan perbaikan sebelum API
> boleh dipublikasikan ke production.

### Phase 1: Setup & Data ✅ Selesai dan terverifikasi
- [x] `go mod init github.com/beon/bin-api`
- [x] Scaffold struktur folder project Go
- [x] Buat migration SQL schema (`migrations/001_create_bins.sql`)
- [x] Buat CLI importer (`cmd/importer/main.go`) — mapping kolom CSV venelinkochev sudah benar
- [x] Buat `docker-compose.yml` dengan service PostgreSQL
- [x] Buat `.env.example`
- [x] Buat `scripts/import_csv.sh` (download + import otomatis)
- [x] Dataset lokal tersedia: 374.788 record, seluruhnya BIN 6 digit
- [x] Verifikasi `docker compose up -d postgres` dan PostgreSQL healthcheck
- [x] Import atomik dataset ke database aktif: 374.788 row
- [x] Verifikasi data masuk: `GET /api/v1/stats` mengembalikan 374.788

### Phase 2: Core API ✅ Selesai dan smoke-tested
- [x] Setup Gin framework (`cmd/api/main.go`)
- [x] Buat layer repository PostgreSQL (`internal/repository/postgres/`)
- [x] Buat service layer dengan lookup logic (`internal/service/bin_service.go`)
- [x] Handler `GET /api/v1/bin/:number` dan `GET /api/v1/stats`
- [x] Handler legacy full-PAN telah dihapus pada Phase 6; Luhn diarahkan ke client-side
- [x] Integrasi fallback ke binlist.net jika BIN tidak ada di DB
- [x] Test manual dengan curl setelah DB terisi: health, stats, 6 digit, fallback 8→6, invalid 7 digit

### Phase 3: Caching & Performance 🔄 Cache dasar sudah ada
- [x] In-memory cache TTL 30 menit (`pkg/cache/cache.go`)
- [x] Batasi kapasitas cache agar tidak tumbuh tanpa batas
- [x] Tambahkan negative cache dan `singleflight` untuk lookup yang sama
- [ ] **⬜ Benchmark test (target 1000+ req/detik)**
- [x] Tambahkan rate limiting per client dan global
- [ ] ⬜ Opsional: integrasi Redis untuk multi-instance

### Phase 4: Auth & Admin 🔄 Middleware ada, endpoint belum ada
- [x] API Key authentication via header `X-API-Key` (`internal/middleware/auth.go`)
- [x] Validasi Luhn algorithm (`pkg/validator/card.go`)
- [ ] **⬜ Endpoint admin `POST /api/v1/admin/import` untuk re-import data**
- [ ] **⬜ Batch lookup endpoint `POST /api/v1/bins/batch`**

### Phase 5: Deployment ✅ Dockerfile & Compose sudah ada
- [x] `Dockerfile` (multi-stage build)
- [x] `docker-compose.yml` (API + PostgreSQL)
- [x] Test `docker compose up` full stack untuk health, removed route, dan rate limit
- [ ] **⬜ Setup nginx reverse proxy**
- [ ] **⬜ Deploy ke VPS**
- [ ] **⬜ Setup domain + SSL**

---

### Phase 6: Security Gate P0 ✅ Implementasi selesai

- [x] Hapus endpoint GET yang menerima full PAN/card number melalui query string
- [x] Tetapkan Luhn sebagai client-side check; server hanya menerima BIN/IIN
- [x] Konfigurasikan logger Gin agar tidak mencatat query string, termasuk pada route 404
- [x] Patch `github.com/quic-go/quic-go` dari `v0.59.0` ke `v0.59.1`
- [x] Jalankan ulang `govulncheck ./...`: 0 vulnerability reachable
- [x] Jalankan `gosec ./...`: 0 issue
- [x] Tambahkan `.dockerignore` untuk `.env`, `.git`, `data/*.csv`, binary, dan artefak development
- [x] Hilangkan fallback credential; `DATABASE_URL` wajib dan aplikasi fail-fast jika kosong
- [x] Hapus default password dari Compose; password wajib diberikan melalui environment
- [x] Tambahkan rate limit token bucket per client dan global untuk endpoint publik
- [x] Jadikan enrichment default nonaktif; hanya aktif jika operator mengaturnya eksplisit
- [x] Batasi binlist.net menjadi 5 request/jam dengan burst 5 sesuai limit upstream
- [x] Tambahkan timeout, circuit breaker, validasi respons, negative cache, dan `singleflight`
- [x] Batasi positive cache 20.000 dan negative cache 50.000 entry
- [ ] Saat deployment: isi password/secret production yang kuat dan batasi permission `.env`

**Exit criteria Phase 6:** tidak ada full PAN di URL/log, tidak ada vulnerability
reachable, secret tidak masuk Docker context, dan request acak tidak dapat membanjiri
binlist.net maupun database.

### Phase 7: Correctness & Data Integrity P1 ✅ Implementasi selesai

- [x] Validator hanya menerima digit ASCII dengan panjang tepat 6 atau 8 digit, bukan 7 digit
- [x] Implementasikan longest-prefix lookup: exact 8 digit lalu fallback ke 6 digit
- [x] Route lookup memvalidasi dan memakai parameter `:number`; route full PAN tetap dihapus
- [x] Tegaskan bahwa Luhn hanya checksum, bukan bukti kartu aktif/valid untuk transaksi
- [x] Validasi header, jumlah kolom, format BIN, kode negara, dan panjang field saat import CSV
- [x] Importer exit non-zero saat gagal dan melaporkan jumlah row yang benar-benar di-commit
- [x] Gunakan transaction/staging table, advisory lock, dan rollback untuk refresh atomic
- [x] Hapus stale row hanya jika `source = 'local'`; hasil enrichment dipertahankan
- [x] Validasi/normalisasi respons enrichment sebelum `Upsert`; unknown tetap `NULL`
- [x] Evaluasi sumber: dataset lama tetap best-effort, bukan authoritative untuk keputusan finansial
- [x] Simpan source, URL, commit version, SHA-256, timestamp, status, dan row counts pada setiap refresh

**Exit criteria Phase 7:** lookup 6/8 digit konsisten, proses import tidak dapat
melaporkan sukses palsu, dan asal serta umur data dapat diaudit.

### Phase 7.1: Private Multi-Provider Enrichment ✅ Implementasi selesai

- [x] Urutan fallback: PostgreSQL → HandyAPI → binlist.net → 404/503
- [x] HandyAPI hanya aktif jika `HANDY_API_KEY` tersedia di environment
- [x] Respons Handy divalidasi dan dinormalisasi sebelum disimpan sebagai `source = 'handy_api'`
- [x] Respons valid wajib berhasil di-`Upsert` sebelum dikembalikan ke client
- [x] Tambahkan quota guard Handy bulanan atomik di PostgreSQL, default 3.000 request
- [x] `404`, `429`, quota habis, invalid response, dan provider error meneruskan request ke Binlist
- [x] Pertahankan timeout, rate limiter lokal, circuit breaker, dan batas response body pada kedua provider
- [x] Terima variasi schema Handy `Country` berupa object atau array kosong
- [x] Tolak DAXXTEAM offline DB: script sumber menghasilkan BIN/bank/country/type secara acak, bukan data terverifikasi
- [x] Dokumentasikan keputusan operator untuk penggunaan private dan penyimpanan hasil HandyAPI

**Keputusan operator 9 September 2026:** service dipakai di area private dan
hasil HandyAPI disimpan untuk mencegah lookup berulang. Ketentuan HandyAPI tetap
membatasi caching/storage pada kebijakan publiknya; pemilik deployment menerima
keputusan ini dan bertanggung jawab memperoleh izin yang diperlukan.

### Phase 8: Automated Tests & Code Quality P1 ✅ Implementasi dan verifikasi lokal selesai

- [x] Jalankan `gofmt` pada seluruh file Go
- [x] Jalankan `go mod tidy` agar direct/indirect dependency benar
- [x] Unit test validator: ASCII-only, panjang, Luhn valid/invalid, dan edge cases
- [x] Unit test cache mencakup miss, update, expiry, minimum capacity, dan eviction
- [x] Unit test service mencakup cache, urutan fallback, quota, persistence, 404, dan 503
- [x] Handler test dengan `httptest` mencakup kontrak 200/400/404/500/503 dan stats
- [x] Tambahkan regression test agar PAN/query string tidak muncul di access log
- [x] Integration test repository/importer dengan schema PostgreSQL disposable
- [x] Rebuild Staticcheck 2026.1 (`v0.7.0`) memakai Go 1.26 dan verifikasi tanpa temuan
- [x] Tambahkan CI: format, module consistency, Actionlint, Vet, Staticcheck, race/integration test, coverage, build, dan Govulncheck
- [x] Tetapkan dan enforce target coverage setiap paket kritis minimal 80%

**Exit criteria Phase 8:** CI hijau dan jalur keamanan/correctness utama memiliki
regression test. Seluruh quality gate sudah hijau lokal; status run GitHub pertama
baru tersedia setelah perubahan di-commit dan di-push.

### Phase 9: Runtime & Observability P1 ✅ Implementasi dan smoke test selesai

- [x] Propagasikan `context.Context` dari handler ke service, repository, dan HTTP client
- [x] Gunakan `http.Server` dengan read-header/read/write/idle timeout dan batas header
- [x] Tambahkan graceful shutdown untuk HTTP server, cache worker, dan koneksi database
- [x] Konfigurasi DB pool: max open/idle connection, max lifetime, dan max idle time
- [x] Pisahkan liveness `/live` dan readiness `/ready`; readiness memakai `PingContext`
- [x] Jalankan embedded migration berversi dengan checksum, transaction, dan advisory lock saat API/importer startup
- [x] Hapus index `idx_bins_bin` yang duplikat setelah constraint unique diverifikasi migration 004
- [x] Tambahkan JSON structured logging, request ID, redaksi PAN/query, Prometheus metrics, dan alert rules
- [x] Bedakan respons not-found `404`, upstream rate-limit `429`, dan operational failure `503`

**Exit criteria Phase 9:** shutdown bersih, dependency failure terlihat dari readiness
dan metrics, serta request yang dibatalkan tidak terus memakai DB/upstream. Seluruh
criteria telah diuji pada Compose stack lokal.

### Phase 10: Deployment & Documentation P2 ⬜

- [x] Samakan versi Go pada `go.mod`, Dockerfile, README, dan tool lokal; CI memakai `go-version-file`
- [x] Parameterkan port Compose agar mengikuti `.env`; tambahkan PostgreSQL healthcheck dan dependency condition
- [x] Hapus top-level Compose `version` yang sudah obsolete
- [x] Jalankan container sebagai non-root; pin digest tetap menunggu kebijakan update
- [x] Dokumentasikan `CORS_ORIGINS`, gunakan default fail-closed, dan tambahkan `Vary: Origin`
- [x] Hapus klaim fitur admin yang belum tersedia dari README
- [x] Perbaiki contoh Nginx `proxy_pass` dan nonaktifkan access log query pada location API
- [x] Koreksi dokumentasi cache (`map` + mutex, bukan `sync.Map`)
- [ ] Tambahkan file `LICENSE` karena README menyatakan MIT
- [x] Uji `docker compose up` full stack dan smoke test seluruh endpoint

**Exit criteria Phase 10:** satu perintah dapat menjalankan full stack tanpa konflik
konfigurasi, dokumentasi sesuai implementasi, dan image production lolos smoke test.

---

### Urutan Langkah Selanjutnya (hasil audit 9 September 2026)

```
1. ✅ Phase 6 security gate selesai
2. ✅ Phase 7 correctness dan data integrity selesai
3. ✅ Phase 7.1 fallback private HandyAPI → Binlist dan persistent enrichment selesai
4. ✅ Phase 8 automated tests, coverage gate, Staticcheck, dan CI selesai lokal
5. ✅ Phase 9 runtime hardening dan observability selesai
6. Mulai Phase 10 deployment/documentation finalization
7. Isi secret production, jalankan smoke test final, lalu deploy ke VPS
```

---

## 9. Estimasi Biaya

| Item | Biaya |
|---|---|
| BIN Dataset (GitHub open-source) | **Rp 0** |
| HandyAPI + binlist.net fallback | **Rp 0** (quota/rate limited) |
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

Strategi update setelah remediation:

1. **Scheduled refresh**: Download commit immutable, verifikasi checksum/schema, lalu import secara atomic melalui staging table.
2. **Controlled enrichment**: Default contoh tetap nonaktif. Pada deployment private, urutan fallback adalah HandyAPI (guard 3.000/bulan) lalu Binlist (5/jam); hasil valid disimpan dengan provenance provider.
3. **Source monitoring**: Pantau repo `venelinkochev/bin-list-data`; data 6 digit terakhir diperbarui 11 Februari 2025 dan tidak cukup untuk akurasi IIN 8 digit modern.
4. **Audit trail**: Tabel `dataset_imports` menyimpan sumber, URL, versi, SHA-256, waktu, status, error, serta jumlah read/import/delete.

### Keputusan sumber data Phase 7

- Dataset `venelinkochev/bin-list-data` dipertahankan hanya sebagai baseline gratis/best-effort karena lisensinya jelas (CC BY 4.0), tetapi freshness-nya tidak memenuhi standar untuk keputusan finansial production.
- Binlist.net hanya dipakai sebagai enrichment opsional. Dokumentasi resminya menyebut data tidak sempurna, free limit 5 request/jam, dan dukungan 8 digit tanpa batas berada pada layanan premium.
- Sebelum API dipakai untuk fraud/routing/otorisasi pembayaran, pilih provider berlisensi yang mempunyai SLA, pembaruan 8-digit IIN, dan hak penggunaan data yang sesuai. Luhn tidak menggantikan verifikasi processor.
- DAXXTEAM tidak boleh diimpor: audit `build_database.py` menunjukkan record dibentuk dengan random BIN range serta pemilihan acak untuk bank, negara, type, tier, dan prepaid.
- HandyAPI dipilih operator sebagai fallback pertama untuk lingkungan private, diikuti Binlist. Hasil disimpan agar lookup berikutnya berasal dari PostgreSQL.

---

## 12. Keamanan API

### Kondisi setelah Phase 6 — 8 September 2026

- [x] Endpoint publik dilindungi global dan per-client rate limiting
- [x] API tidak menerima full PAN dan logger aplikasi tidak mencatat query string
- [x] Enrichment default nonaktif serta dilindungi outbound limiter/circuit breaker jika diaktifkan
- [x] CORS fail-closed ketika `CORS_ORIGINS` kosong; wildcard harus dipilih eksplisit
- [x] `.env` tidak di-track Git
- [x] `.env`, `.git`, dan dataset dikecualikan dari Docker build context
- [x] Middleware API key tersedia, tetapi belum ada endpoint admin yang memakai fungsinya

### Target keamanan

- Tidak menerima atau mencatat full PAN kecuali use case telah direview khusus
- Semua endpoint publik memiliki rate limit dan batas resource
- Endpoint admin wajib authentication yang fail-closed dan secret production yang kuat
- Log memiliki request ID tetapi selalu meredaksi query/body/header sensitif
- Docker image dan build cache tidak mengandung `.env` atau dataset development
- `govulncheck` dan security checks berjalan otomatis di CI

---

## 13. Baseline Audit 8 September 2026

| Pemeriksaan | Hasil audit |
|---|---|
| Git | Branch `main`; ada perubahan lokal indentasi di `cmd/api/main.go`, tidak diubah oleh audit |
| Build | `go build ./...` lulus dengan Go lokal 1.26.3 |
| Vet | `go vet ./...` lulus |
| Race test | Lulus, tetapi tidak ada file test |
| Coverage | 0% pada seluruh package |
| Module integrity | `go mod verify` lulus |
| Module hygiene | `go mod tidy -diff` menemukan `go.mod`/`go.sum` belum tidy |
| Formatting | `gofmt -d` menemukan tiga file belum terformat |
| Staticcheck | Belum dapat berjalan karena binary dibuat dengan Go 1.25, module meminta Go 1.26.2 |
| Vulnerability | 1 reachable: `GO-2026-5676` pada `quic-go v0.59.0`, fixed `v0.59.1` |
| Compose | Config valid, tetapi top-level `version` obsolete dan port hardcoded |
| Port lokal | Compose `5432`/`8080` bentrok dengan container lain; `.env` `5434`/`8828` tidak dipakai mapping Compose |
| Dataset lokal | 374.788 record valid; semuanya BIN 6 digit |
| Dataset upstream | Blob lokal sama dengan upstream; update data terakhir 11 Februari 2025 |
| Secret scan dasar | Tidak ditemukan `.env` atau credential production yang ter-track; default credential masih ada di contoh/Compose |

### Verifikasi setelah implementasi Phase 6

| Pemeriksaan | Hasil |
|---|---|
| Full-PAN route | Sudah dihapus; smoke test endpoint lama menghasilkan `404` |
| Log redaction | Query string dinonaktifkan pada Gin logger; sentinel PAN tidak muncul pada log smoke test |
| Public rate limit | Terverifikasi menghasilkan `429` setelah burst per-client habis |
| Enrichment | Default `false`; local limiter 5 request/jam, burst 5, plus circuit breaker |
| Cache abuse guard | Positive cache maksimal 20.000 dan negative cache maksimal 50.000 entry |
| Dependency scan | `govulncheck ./...`: 0 vulnerability reachable |
| Static security scan | `gosec ./...`: 0 issue |
| Automated test | `go test -race -cover ./...` lulus; package baru: middleware 80%, service 51,2%, binlist 80,8%, cache 75% |
| Build checks | `go build ./...` dan `go vet ./...` lulus |
| Docker context | Sekitar 21 KB; `.env` dan dataset 27 MB tidak ikut build context |
| Docker image | Build lulus, Go 1.26.3, Alpine 3.23, runtime user non-root `app` |
| Full-stack smoke | Compose PostgreSQL sehat, API health `200`, limiter `429`, lalu container dihentikan tanpa menghapus volume |

### Verifikasi setelah implementasi Phase 7

| Pemeriksaan | Hasil |
|---|---|
| Import dataset | Sukses atomik: 374.788 read/imported, 0 stale deleted |
| Provenance | Commit `023a4f68ab1b5d7edd45f03acb132f88a49db96a`, SHA-256 `0583860988c7b15d2921025ee8afe89eaf4947fe31acde8ffccfc82691d13e35` |
| Integritas DB | 374.788 BIN unik, semuanya 6 digit, 0 duplicate, constraint ASCII 6/8 aktif |
| Rollback | Dataset uji duplikat exit non-zero; jumlah live tetap 374.788 dan status gagal tercatat |
| Lookup | 6 digit `200`; 8 digit exact diprioritaskan, fallback 8→6 `200`; 7 digit `400` |
| Unknown value | `prepaid`, latitude, dan longitude dataset lokal keluar sebagai JSON `null` |
| API smoke | `/api/v1/health` dan `/api/v1/stats` `200`; stats tepat 374.788 |

### Verifikasi Private Multi-Provider

| Pemeriksaan | Hasil |
|---|---|
| Urutan provider | Unit test membuktikan Handy dipanggil sebelum Binlist |
| Persistent enrichment | Hasil Handy dan Binlist disimpan dengan nilai `source` masing-masing |
| Live Handy smoke | Miss lokal `22217000` menghasilkan `200`, tersimpan sebagai `source=handy_api`, request kedua `200` tanpa kenaikan counter |
| Provider failure | Handy `429/error` otomatis meneruskan lookup ke Binlist |
| Monthly quota | Counter atomik PostgreSQL mencegah request Handy setelah batas tercapai |
| Secret | `HANDY_API_KEY` hanya dibaca dari environment dan tidak masuk log/error |
| DAXXTEAM | Ditolak: 9.639 row sintetis; 1.569 tampak baru tetapi generator datanya random |

### Verifikasi setelah implementasi Phase 8

| Pemeriksaan | Hasil |
|---|---|
| Handler contract | `httptest` mencakup 200, 400, 404, 500, 503, dan stats; coverage 100% |
| Sensitive logging | PAN-like path 12+ digit menjadi `[REDACTED]`; query string tidak dicatat |
| PostgreSQL integration | Schema disposable; lookup exact/fallback, constraint, quota concurrent, import atomik, rollback, dan provenance lulus race test |
| Coverage gate | Handler 100%; middleware 83,1%; observability 100%; request context 100%; service 86,4%; migration 81,2%; repository 80,3%; Binlist 80,8%; cache 85,7%; Handy 81,4%; validator 90% |
| Static analysis | Vet dan Staticcheck 2026.1 (`v0.7.0`, dibangun dengan Go 1.26.3) lulus tanpa temuan |
| Workflow | Actionlint `v1.7.12` memvalidasi `.github/workflows/ci.yml`; first remote run menunggu commit/push |

### Verifikasi setelah implementasi Phase 9

| Pemeriksaan | Hasil |
|---|---|
| Versioned migration | Empat migration tercatat di `schema_migrations`; rerun idempotent dan checksum drift ditolak |
| Index cleanup | `idx_bins_bin` terhapus; unique constraint/index `bins_bin_key` tetap aktif |
| Runtime probe | DB down: `/live` `200`, `/ready` `503`, gauge `0`; DB pulih: `/ready` `200`, gauge `1` tanpa restart API |
| Graceful shutdown | Compose SIGTERM menghasilkan event JSON `shutdown started` lalu `shutdown complete` sebelum deadline |
| Request correlation | Response dan JSON access log membawa request ID yang sama; request ID tidak aman diganti otomatis |
| Sensitive logging | Query string tidak dicatat dan digit 12+ pada path/error menjadi `[REDACTED]` |
| Metrics | `/metrics` mengekspor request counter, duration histogram, Go/process metrics, dan readiness tanpa label BIN mentah |
| Alert rules | Rule service down, not-ready, error rate >5%, dan p95 latency >2 detik tersedia untuk Prometheus |
| Error contract | Regression test membedakan provider miss `404`, final rate-limit `429` + `Retry-After`, dan failure `503` |

Referensi audit:

- [Go vulnerability GO-2026-5676](https://pkg.go.dev/vuln/GO-2026-5676)
- [PCI SSC — Best Practices for Securing E-commerce](https://listings.pcisecuritystandards.org/pdfs/best_practices_securing_ecommerce.pdf)
- [Docker Compose: top-level `version` obsolete](https://docs.docker.com/reference/compose-file/version-and-name/)
- [Commit terakhir dataset upstream](https://github.com/venelinkochev/bin-list-data/commit/023a4f68ab1b5d7edd45f03acb132f88a49db96a)
- [Binlist.net — batas, akurasi, dan dukungan 8 digit](https://binlist.net/)
- [ISO — perubahan IIN dari 6 menjadi 8 digit](https://www.iso.org/news/2016/11/Ref2146.html)
- [Prometheus — alerting rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/)
- [Prometheus Go client v1.24.1](https://github.com/prometheus/client_golang/releases/tag/v1.24.1)

---

*Dokumen ini menjadi sumber acuan phase perbaikan dan harus diperbarui setiap kali checklist selesai.*
