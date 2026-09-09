package database

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func TestMigrateAppliesOnceAndDetectsDrift(t *testing.T) {
	db := migrationTestDB(t)
	ctx := context.Background()

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("first migration run: %v", err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("idempotent migration run: %v", err)
	}

	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM schema_migrations`); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 4 {
		t.Fatalf("migration count = %d, want 4", count)
	}
	var redundantIndex bool
	if err := db.Get(&redundantIndex, `SELECT to_regclass('idx_bins_bin') IS NOT NULL`); err != nil {
		t.Fatalf("check redundant index: %v", err)
	}
	if redundantIndex {
		t.Fatal("idx_bins_bin still exists")
	}

	if _, err := db.Exec(`UPDATE schema_migrations SET checksum = repeat('0', 64) WHERE version = 1`); err != nil {
		t.Fatalf("tamper migration checksum: %v", err)
	}
	if err := Migrate(ctx, db); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("drift error = %v, want checksum mismatch", err)
	}
}

func TestLoadMigrationsIsOrdered(t *testing.T) {
	items, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("migration count = %d, want 4", len(items))
	}
	for index, item := range items {
		want := int64(index + 1)
		if item.version != want || item.checksum == "" {
			t.Fatalf("migration %d = %+v", index, item)
		}
	}
}

func TestMigrateAndApplyFailurePaths(t *testing.T) {
	db := migrationTestDB(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Migrate(cancelled, db); err == nil {
		t.Fatal("migration accepted a cancelled context")
	}

	ctx := context.Background()
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("prepare migrations: %v", err)
	}
	connection, err := db.Connx(ctx)
	if err != nil {
		t.Fatalf("acquire test connection: %v", err)
	}
	invalid := migration{version: 99, name: "099_invalid.sql", contents: []byte("INVALID SQL"), checksum: strings.Repeat("a", 64)}
	if err := applyMigration(ctx, connection, invalid); err == nil || !strings.Contains(err.Error(), "apply migration") {
		t.Fatalf("invalid SQL error = %v", err)
	}
	if _, err := connection.ExecContext(ctx, `DROP TABLE schema_migrations`); err != nil {
		t.Fatalf("drop migration metadata: %v", err)
	}
	if err := applyMigration(ctx, connection, invalid); err == nil || !strings.Contains(err.Error(), "check migration") {
		t.Fatalf("metadata lookup error = %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close test connection: %v", err)
	}
	if err := applyMigration(ctx, connection, invalid); err == nil || !strings.Contains(err.Error(), "begin migration") {
		t.Fatalf("closed connection error = %v", err)
	}
}

func TestMigrateSerializesConcurrentInstances(t *testing.T) {
	db := migrationTestDB(t)
	ctx := context.Background()
	var group sync.WaitGroup
	errorsFound := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			errorsFound <- Migrate(ctx, db)
		}()
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatalf("concurrent migration: %v", err)
		}
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM schema_migrations`); err != nil || count != 4 {
		t.Fatalf("migration count = %d, error=%v; want 4", count, err)
	}
}

func migrationTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	admin, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	schema := fmt.Sprintf("beon_migration_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema)); err != nil {
		admin.Close()
		t.Fatalf("create migration schema: %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sqlx.Connect("postgres", parsed.String())
	if err != nil {
		t.Fatalf("connect migration schema: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = admin.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`)
		_ = admin.Close()
	})
	return db
}
