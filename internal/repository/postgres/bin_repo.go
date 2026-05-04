package postgres

import (
"database/sql"
"errors"
"fmt"

"github.com/beon/bin-api/internal/model"
"github.com/jmoiron/sqlx"
)

type binRepository struct {
db *sqlx.DB
}

// New returns a PostgreSQL-backed BINRepository
func New(db *sqlx.DB) *binRepository {
return &binRepository{db: db}
}

func (r *binRepository) GetByBIN(bin string) (*model.BIN, error) {
var b model.BIN
const q = `
SELECT id, bin, brand, type, category,
       bank_name, bank_url, bank_phone,
       country_name, country_code, country_currency,
       country_latitude, country_longitude,
       prepaid, source, created_at, updated_at
FROM bins
WHERE bin = $1
LIMIT 1`
err := r.db.Get(&b, q, bin)
if errors.Is(err, sql.ErrNoRows) {
return nil, nil
}
return &b, err
}

func (r *binRepository) Upsert(b *model.BIN) error {
const q = `
INSERT INTO bins (
bin, brand, type, category,
bank_name, bank_url, bank_phone,
country_name, country_code, country_currency,
country_latitude, country_longitude,
prepaid, source, updated_at
) VALUES (
:bin, :brand, :type, :category,
:bank_name, :bank_url, :bank_phone,
:country_name, :country_code, :country_currency,
:country_latitude, :country_longitude,
:prepaid, :source, NOW()
)
ON CONFLICT (bin) DO UPDATE SET
brand            = EXCLUDED.brand,
type             = EXCLUDED.type,
category         = EXCLUDED.category,
bank_name        = EXCLUDED.bank_name,
bank_url         = EXCLUDED.bank_url,
bank_phone       = EXCLUDED.bank_phone,
country_name     = EXCLUDED.country_name,
country_code     = EXCLUDED.country_code,
country_currency = EXCLUDED.country_currency,
country_latitude = EXCLUDED.country_latitude,
country_longitude= EXCLUDED.country_longitude,
prepaid          = EXCLUDED.prepaid,
source           = EXCLUDED.source,
updated_at       = NOW()`
_, err := r.db.NamedExec(q, b)
return err
}

func (r *binRepository) BatchUpsert(bins []*model.BIN) error {
if len(bins) == 0 {
return nil
}
const chunkSize = 500
for i := 0; i < len(bins); i += chunkSize {
end := i + chunkSize
if end > len(bins) {
end = len(bins)
}
chunk := bins[i:end]
records := make([]model.BIN, 0, len(chunk))
for _, b := range chunk {
records = append(records, *b)
}
if _, err := r.db.NamedExec(batchUpsertQuery, records); err != nil {
return fmt.Errorf("batch upsert chunk %d: %w", i/chunkSize, err)
}
}
return nil
}

const batchUpsertQuery = `
INSERT INTO bins (
bin, brand, type, category,
bank_name, bank_url, bank_phone,
country_name, country_code, country_currency,
country_latitude, country_longitude,
prepaid, source, updated_at
) VALUES (
:bin, :brand, :type, :category,
:bank_name, :bank_url, :bank_phone,
:country_name, :country_code, :country_currency,
:country_latitude, :country_longitude,
:prepaid, :source, NOW()
)
ON CONFLICT (bin) DO UPDATE SET
brand             = EXCLUDED.brand,
type              = EXCLUDED.type,
category          = EXCLUDED.category,
bank_name         = EXCLUDED.bank_name,
bank_url          = EXCLUDED.bank_url,
bank_phone        = EXCLUDED.bank_phone,
country_name      = EXCLUDED.country_name,
country_code      = EXCLUDED.country_code,
country_currency  = EXCLUDED.country_currency,
country_latitude  = EXCLUDED.country_latitude,
country_longitude = EXCLUDED.country_longitude,
prepaid           = EXCLUDED.prepaid,
source            = EXCLUDED.source,
updated_at        = NOW()`

func (r *binRepository) Count() (int64, error) {
var count int64
err := r.db.Get(&count, `SELECT COUNT(*) FROM bins`)
return count, err
}
