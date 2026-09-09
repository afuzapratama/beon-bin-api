package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/beon/bin-api/internal/model"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

// New returns a PostgreSQL-backed BINRepository
func New(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetByBIN(ctx context.Context, bin string) (*model.BIN, error) {
	var b model.BIN
	const q = `
SELECT id, bin, brand, type, category,
       bank_name, bank_url, bank_phone,
       country_name, country_code, country_currency,
       country_latitude, country_longitude,
       prepaid, source, created_at, updated_at
FROM bins
WHERE bin = $1
   OR (LENGTH($1::text) = 8 AND bin = LEFT($1::text, 6))
ORDER BY LENGTH(bin) DESC
LIMIT 1`
	err := r.db.GetContext(ctx, &b, q, bin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &b, err
}

func (r *Repository) Upsert(ctx context.Context, b *model.BIN) error {
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
	_, err := r.db.NamedExecContext(ctx, q, b)
	return err
}

func (r *Repository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM bins`)
	return count, err
}

// TryConsumeEnrichmentQuota atomically reserves one provider request for a
// calendar period. It coordinates the limit across restarts and instances.
func (r *Repository) TryConsumeEnrichmentQuota(
	ctx context.Context,
	provider string,
	periodStart time.Time,
	limit int64,
) (bool, error) {
	if limit <= 0 {
		return false, nil
	}
	const q = `
WITH consumed AS (
    INSERT INTO enrichment_usage (provider, period_start, request_count, updated_at)
    VALUES ($1, $2, 1, NOW())
    ON CONFLICT (provider, period_start) DO UPDATE SET
        request_count = enrichment_usage.request_count + 1,
        updated_at = NOW()
    WHERE enrichment_usage.request_count < $3
    RETURNING request_count
)
SELECT EXISTS (SELECT 1 FROM consumed)`
	var consumed bool
	if err := r.db.GetContext(ctx, &consumed, q, provider, periodStart, limit); err != nil {
		return false, fmt.Errorf("reserve %s quota: %w", provider, err)
	}
	return consumed, nil
}

// DatasetImportMetadata describes the source and outcome of a dataset refresh.
type DatasetImportMetadata struct {
	Source         string
	SourceURL      string
	SourceVersion  string
	ChecksumSHA256 string
	RowsRead       int64
}

// DatasetImportResult reports the atomically committed dataset changes.
type DatasetImportResult struct {
	RowsImported int64
	RowsDeleted  int64
}

// DatasetImport stages a complete local dataset inside one transaction.
type DatasetImport struct {
	tx        *sqlx.Tx
	startedAt time.Time
}

// BeginDatasetImport starts an isolated, single-writer dataset refresh.
func (r *Repository) BeginDatasetImport(ctx context.Context) (*DatasetImport, error) {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin dataset import: %w", err)
	}

	const setup = `
SELECT pg_advisory_xact_lock(hashtext('beon-bin-api:dataset-import'));
CREATE TEMP TABLE bins_import_stage (
    bin               VARCHAR(8) PRIMARY KEY,
    brand             VARCHAR(50),
    type              VARCHAR(20),
    category          VARCHAR(50),
    bank_name         VARCHAR(200),
    bank_url          VARCHAR(200),
    bank_phone        VARCHAR(100),
    country_name      VARCHAR(100),
    country_code      VARCHAR(3),
    country_currency  VARCHAR(10),
    country_latitude  DECIMAL(9,6),
    country_longitude DECIMAL(9,6),
    prepaid           BOOLEAN,
    source            VARCHAR(50) NOT NULL,
    CONSTRAINT bins_import_stage_bin_format CHECK (bin ~ '^([0-9]{6}|[0-9]{8})$')
) ON COMMIT DROP;`
	if _, err := tx.ExecContext(ctx, setup); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("prepare dataset import: %w", err)
	}

	return &DatasetImport{tx: tx, startedAt: time.Now().UTC()}, nil
}

// Stage validates database constraints and stages a batch without changing live data.
func (i *DatasetImport) Stage(ctx context.Context, bins []*model.BIN) error {
	if len(bins) == 0 {
		return nil
	}
	records := make([]model.BIN, 0, len(bins))
	for _, b := range bins {
		records = append(records, *b)
	}
	if _, err := i.tx.NamedExecContext(ctx, stageUpsertQuery, records); err != nil {
		return fmt.Errorf("stage dataset batch: %w", err)
	}
	return nil
}

const stageUpsertQuery = `
INSERT INTO bins_import_stage (
bin, brand, type, category,
bank_name, bank_url, bank_phone,
country_name, country_code, country_currency,
country_latitude, country_longitude,
prepaid, source
) VALUES (
:bin, :brand, :type, :category,
:bank_name, :bank_url, :bank_phone,
:country_name, :country_code, :country_currency,
:country_latitude, :country_longitude,
:prepaid, :source
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
source            = EXCLUDED.source`

// Commit atomically publishes staged rows, removes stale local rows, and records provenance.
func (i *DatasetImport) Commit(ctx context.Context, metadata DatasetImportMetadata) (DatasetImportResult, error) {
	var result DatasetImportResult
	if err := i.tx.GetContext(ctx, &result.RowsImported, `SELECT COUNT(*) FROM bins_import_stage`); err != nil {
		return result, fmt.Errorf("count staged rows: %w", err)
	}
	if result.RowsImported != metadata.RowsRead {
		return result, fmt.Errorf("dataset contains duplicate BINs: read %d rows but staged %d unique rows", metadata.RowsRead, result.RowsImported)
	}

	const merge = `
INSERT INTO bins (
    bin, brand, type, category,
    bank_name, bank_url, bank_phone,
    country_name, country_code, country_currency,
    country_latitude, country_longitude,
    prepaid, source, updated_at
)
SELECT bin, brand, type, category,
       bank_name, bank_url, bank_phone,
       country_name, country_code, country_currency,
       country_latitude, country_longitude,
       prepaid, source, NOW()
FROM bins_import_stage
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
	if _, err := i.tx.ExecContext(ctx, merge); err != nil {
		return result, fmt.Errorf("publish staged rows: %w", err)
	}

	deleteResult, err := i.tx.ExecContext(ctx, `
DELETE FROM bins AS existing
WHERE existing.source = 'local'
  AND NOT EXISTS (
      SELECT 1 FROM bins_import_stage AS staged WHERE staged.bin = existing.bin
  )`)
	if err != nil {
		return result, fmt.Errorf("delete stale local rows: %w", err)
	}
	result.RowsDeleted, err = deleteResult.RowsAffected()
	if err != nil {
		return result, fmt.Errorf("count deleted local rows: %w", err)
	}

	const recordSuccess = `
INSERT INTO dataset_imports (
    source, source_url, source_version, checksum_sha256,
    status, rows_read, rows_imported, rows_deleted,
    started_at, completed_at
) VALUES ($1, $2, $3, $4, 'succeeded', $5, $6, $7, $8, NOW())`
	if _, err := i.tx.ExecContext(ctx, recordSuccess,
		metadata.Source, metadata.SourceURL, metadata.SourceVersion, metadata.ChecksumSHA256,
		metadata.RowsRead, result.RowsImported, result.RowsDeleted, i.startedAt,
	); err != nil {
		return result, fmt.Errorf("record dataset import: %w", err)
	}

	if err := i.tx.Commit(); err != nil {
		return result, fmt.Errorf("commit dataset import: %w", err)
	}
	return result, nil
}

// Rollback discards all staged data. It is safe to call after a failed commit.
func (i *DatasetImport) Rollback() error {
	err := i.tx.Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		return nil
	}
	return err
}

// RecordDatasetImportFailure stores a failed attempt outside the rolled-back data transaction.
func (r *Repository) RecordDatasetImportFailure(ctx context.Context, metadata DatasetImportMetadata, importErr error) error {
	message := importErr.Error()
	if len(message) > 2_000 {
		message = message[:2_000]
	}
	const q = `
INSERT INTO dataset_imports (
    source, source_url, source_version, checksum_sha256,
    status, rows_read, rows_imported, rows_deleted,
    error_message, started_at, completed_at
) VALUES ($1, $2, $3, NULLIF($4, ''), 'failed', $5, 0, 0, $6, NOW(), NOW())`
	_, err := r.db.ExecContext(ctx, q,
		metadata.Source, metadata.SourceURL, metadata.SourceVersion, metadata.ChecksumSHA256,
		metadata.RowsRead, message,
	)
	return err
}
