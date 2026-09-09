package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/beon/bin-api/migrations"
	"github.com/jmoiron/sqlx"
)

const migrationLockID int64 = 7_268_295_671_039_112_043

type migration struct {
	version  int64
	name     string
	contents []byte
	checksum string
}

// Migrate applies embedded SQL migrations exactly once and verifies that
// previously applied files have not changed.
func Migrate(ctx context.Context, db *sqlx.DB) error {
	connection, err := db.Connx(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer connection.Close()

	if _, err := connection.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = connection.ExecContext(unlockCtx, `SELECT pg_advisory_unlock($1)`, migrationLockID)
	}()

	if _, err := connection.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     BIGINT      PRIMARY KEY,
    name        TEXT        NOT NULL,
    checksum    CHAR(64)    NOT NULL,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	ordered, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, item := range ordered {
		if err := applyMigration(ctx, connection, item); err != nil {
			return err
		}
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		return nil, fmt.Errorf("list embedded migrations: %w", err)
	}

	ordered := make([]migration, 0, len(entries))
	versions := make(map[int64]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		separator := strings.IndexByte(entry.Name(), '_')
		if separator <= 0 {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.ParseInt(entry.Name()[:separator], 10, 64)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}
		if previous, exists := versions[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %d in %q and %q", version, previous, entry.Name())
		}
		contents, err := migrations.Files.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		sum := sha256.Sum256(contents)
		versions[version] = entry.Name()
		ordered = append(ordered, migration{
			version:  version,
			name:     entry.Name(),
			contents: contents,
			checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].version < ordered[j].version })
	return ordered, nil
}

func applyMigration(ctx context.Context, connection *sqlx.Conn, item migration) error {
	tx, err := connection.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", item.name, err)
	}
	defer tx.Rollback()

	var appliedChecksum string
	err = tx.GetContext(ctx, &appliedChecksum,
		`SELECT checksum FROM schema_migrations WHERE version = $1`, item.version)
	switch {
	case err == nil:
		if strings.TrimSpace(appliedChecksum) != item.checksum {
			return fmt.Errorf("migration %s checksum mismatch", item.name)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit verified migration %s: %w", item.name, err)
		}
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("check migration %s: %w", item.name, err)
	}

	if _, err := tx.ExecContext(ctx, string(item.contents)); err != nil {
		return fmt.Errorf("apply migration %s: %w", item.name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
		item.version, item.name, item.checksum,
	); err != nil {
		return fmt.Errorf("record migration %s: %w", item.name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", item.name, err)
	}
	return nil
}
