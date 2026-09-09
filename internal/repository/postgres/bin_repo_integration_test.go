package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/beon/bin-api/internal/database"
	"github.com/beon/bin-api/internal/model"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func TestRepositoryLookupUpsertCountAndQuota(t *testing.T) {
	repo, db := integrationRepository(t)
	ctx := context.Background()

	base := &model.BIN{BIN: "411111", Brand: "visa", Source: "local"}
	if err := repo.Upsert(ctx, base); err != nil {
		t.Fatalf("upsert base BIN: %v", err)
	}

	fallback, err := repo.GetByBIN(ctx, "41111199")
	if err != nil {
		t.Fatalf("lookup six-digit fallback: %v", err)
	}
	if fallback == nil || fallback.BIN != "411111" {
		t.Fatalf("fallback = %+v, want six-digit BIN", fallback)
	}

	exact := &model.BIN{BIN: "41111199", Brand: "visa", Category: "platinum", Source: "handy_api"}
	if err := repo.Upsert(ctx, exact); err != nil {
		t.Fatalf("upsert exact BIN: %v", err)
	}
	found, err := repo.GetByBIN(ctx, "41111199")
	if err != nil {
		t.Fatalf("lookup exact BIN: %v", err)
	}
	if found == nil || found.BIN != exact.BIN || found.Category != "platinum" {
		t.Fatalf("exact lookup = %+v", found)
	}
	if count, err := repo.Count(ctx); err != nil || count != 2 {
		t.Fatalf("count = %d, error = %v; want 2", count, err)
	}
	if missing, err := repo.GetByBIN(ctx, "999999"); err != nil || missing != nil {
		t.Fatalf("missing lookup = %+v, error = %v", missing, err)
	}
	if err := repo.Upsert(ctx, &model.BIN{BIN: "invalid", Source: "local"}); err == nil {
		t.Fatal("database accepted an invalid BIN")
	}

	const quotaLimit = 5
	period := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	var consumed atomic.Int32
	var quotaErrors atomic.Int32
	var group sync.WaitGroup
	for range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			allowed, quotaErr := repo.TryConsumeEnrichmentQuota(ctx, "handy_api", period, quotaLimit)
			if quotaErr != nil {
				quotaErrors.Add(1)
				return
			}
			if allowed {
				consumed.Add(1)
			}
		}()
	}
	group.Wait()
	if quotaErrors.Load() != 0 || consumed.Load() != quotaLimit {
		t.Fatalf("quota consumed=%d errors=%d, want %d/0", consumed.Load(), quotaErrors.Load(), quotaLimit)
	}
	var stored int
	if err := db.Get(&stored, `SELECT request_count FROM enrichment_usage WHERE provider = 'handy_api'`); err != nil {
		t.Fatalf("read quota counter: %v", err)
	}
	if stored != quotaLimit {
		t.Fatalf("stored quota = %d, want %d", stored, quotaLimit)
	}
}

func TestDatasetImportIsAtomicAndPreservesEnrichment(t *testing.T) {
	repo, db := integrationRepository(t)
	ctx := context.Background()

	for _, record := range []*model.BIN{
		{BIN: "411111", Brand: "old", Source: "local"},
		{BIN: "555555", Brand: "stale", Source: "local"},
		{BIN: "22217000", Brand: "mastercard", Source: "handy_api"},
	} {
		if err := repo.Upsert(ctx, record); err != nil {
			t.Fatalf("seed %s: %v", record.BIN, err)
		}
	}

	batch := []*model.BIN{
		{BIN: "411111", Brand: "visa", Source: "local"},
		{BIN: "400000", Brand: "visa", Source: "local"},
	}
	importer, err := repo.BeginDatasetImport(ctx)
	if err != nil {
		t.Fatalf("begin import: %v", err)
	}
	if err := importer.Stage(ctx, batch); err != nil {
		_ = importer.Rollback()
		t.Fatalf("stage import: %v", err)
	}
	metadata := DatasetImportMetadata{
		Source:         "integration-test",
		SourceURL:      "https://example.test/bins.csv",
		SourceVersion:  "test-commit",
		ChecksumSHA256: strings.Repeat("a", 64),
		RowsRead:       int64(len(batch)),
	}
	result, err := importer.Commit(ctx, metadata)
	if err != nil {
		_ = importer.Rollback()
		t.Fatalf("commit import: %v", err)
	}
	if result.RowsImported != 2 || result.RowsDeleted != 1 {
		t.Fatalf("import result = %+v, want imported=2 deleted=1", result)
	}

	for bin, wantSource := range map[string]string{
		"411111":   "local",
		"400000":   "local",
		"22217000": "handy_api",
	} {
		record, lookupErr := repo.GetByBIN(ctx, bin)
		if lookupErr != nil || record == nil || record.Source != wantSource {
			t.Fatalf("lookup %s = %+v, error=%v; want source %s", bin, record, lookupErr, wantSource)
		}
	}
	if stale, lookupErr := repo.GetByBIN(ctx, "555555"); lookupErr != nil || stale != nil {
		t.Fatalf("stale local row = %+v, error=%v", stale, lookupErr)
	}
	var status string
	if err := db.Get(&status, `SELECT status FROM dataset_imports WHERE source_version = 'test-commit'`); err != nil {
		t.Fatalf("read import audit: %v", err)
	}
	if status != "succeeded" {
		t.Fatalf("import status = %q, want succeeded", status)
	}

	duplicateImport, err := repo.BeginDatasetImport(ctx)
	if err != nil {
		t.Fatalf("begin duplicate import: %v", err)
	}
	duplicate := []*model.BIN{{BIN: "400000", Brand: "visa", Source: "local"}}
	for range 2 {
		if err := duplicateImport.Stage(ctx, duplicate); err != nil {
			_ = duplicateImport.Rollback()
			t.Fatalf("stage duplicate import: %v", err)
		}
	}
	_, err = duplicateImport.Commit(ctx, DatasetImportMetadata{RowsRead: 2})
	if err == nil || !strings.Contains(err.Error(), "duplicate BINs") {
		t.Fatalf("duplicate commit error = %v", err)
	}
	if rollbackErr := duplicateImport.Rollback(); rollbackErr != nil {
		t.Fatalf("rollback duplicate import: %v", rollbackErr)
	}

	sameBatchImport, err := repo.BeginDatasetImport(ctx)
	if err != nil {
		t.Fatalf("begin same-batch duplicate import: %v", err)
	}
	if err := sameBatchImport.Stage(ctx, append(duplicate, duplicate...)); err == nil {
		_ = sameBatchImport.Rollback()
		t.Fatal("same-batch duplicate unexpectedly staged")
	}
	if rollbackErr := sameBatchImport.Rollback(); rollbackErr != nil {
		t.Fatalf("rollback same-batch duplicate import: %v", rollbackErr)
	}

	failure := errors.New(strings.Repeat("x", 2_100))
	if err := repo.RecordDatasetImportFailure(ctx, metadata, failure); err != nil {
		t.Fatalf("record import failure: %v", err)
	}
	var messageLength int
	if err := db.Get(&messageLength, `SELECT char_length(error_message) FROM dataset_imports WHERE status = 'failed'`); err != nil {
		t.Fatalf("read failure audit: %v", err)
	}
	if messageLength != 2_000 {
		t.Fatalf("failure message length = %d, want 2000", messageLength)
	}
}

func integrationRepository(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	schema := fmt.Sprintf("beon_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema)); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}

	parsedDSN, err := url.Parse(dsn)
	if err != nil {
		admin.Close()
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	query := parsedDSN.Query()
	query.Set("search_path", schema)
	parsedDSN.RawQuery = query.Encode()
	db, err := sqlx.Connect("postgres", parsedDSN.String())
	if err != nil {
		_, _ = admin.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`)
		admin.Close()
		t.Fatalf("connect isolated schema: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = admin.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`)
		_ = admin.Close()
	})

	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	return New(db), db
}
