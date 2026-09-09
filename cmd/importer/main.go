package main

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/beon/bin-api/internal/database"
	"github.com/beon/bin-api/internal/model"
	"github.com/beon/bin-api/internal/repository/postgres"
	"github.com/beon/bin-api/pkg/validator"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

const (
	datasetSource = "venelinkochev/bin-list-data"
	datasetURL    = "https://github.com/venelinkochev/bin-list-data"
	batchSize     = 500
)

var expectedHeader = []string{
	"BIN", "Brand", "Type", "Category", "Issuer",
	"IssuerPhone", "IssuerUrl", "isoCode2", "isoCode3", "CountryName",
}

type importOptions struct {
	csvPath       string
	sourceVersion string
	sourceURL     string
}

func main() {
	if err := run(); err != nil {
		log.Printf("import failed: %v", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	options, err := parseFlags(os.Args[1:])
	if err != nil {
		return err
	}
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	return importDataset(ctx, postgres.New(db), options)
}

func parseFlags(args []string) (importOptions, error) {
	var options importOptions
	flags := flag.NewFlagSet("importer", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.sourceVersion, "source-version", "", "upstream commit or immutable dataset version")
	flags.StringVar(&options.sourceURL, "source-url", datasetURL, "dataset source URL")
	if err := flags.Parse(args); err != nil {
		return options, fmt.Errorf("parse arguments: %w", err)
	}
	if flags.NArg() != 1 {
		return options, errors.New("usage: importer -source-version <commit> <data/csv-file>")
	}
	if strings.TrimSpace(options.sourceVersion) == "" {
		return options, errors.New("-source-version is required for an auditable import")
	}
	if len(options.sourceVersion) > 100 {
		return options, errors.New("-source-version must be at most 100 characters")
	}
	options.csvPath = flags.Arg(0)
	return options, nil
}

func importDataset(ctx context.Context, repo *postgres.Repository, options importOptions) (retErr error) {
	file, err := openDatasetFile(options.csvPath)
	if err != nil {
		return err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("checksum CSV: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind CSV: %w", err)
	}
	metadata := postgres.DatasetImportMetadata{
		Source:         datasetSource,
		SourceURL:      options.sourceURL,
		SourceVersion:  options.sourceVersion,
		ChecksumSHA256: hex.EncodeToString(hasher.Sum(nil)),
	}
	session, err := repo.BeginDatasetImport(ctx)
	if err != nil {
		auditCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if auditErr := repo.RecordDatasetImportFailure(auditCtx, metadata, err); auditErr != nil {
			return errors.Join(err, fmt.Errorf("record failed import: %w", auditErr))
		}
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := session.Rollback(); rollbackErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("rollback dataset import: %w", rollbackErr))
		}
		if retErr != nil {
			auditCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if auditErr := repo.RecordDatasetImportFailure(auditCtx, metadata, retErr); auditErr != nil {
				retErr = errors.Join(retErr, fmt.Errorf("record failed import: %w", auditErr))
			}
		}
	}()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = len(expectedHeader)
	reader.ReuseRecord = true

	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read CSV header: %w", err)
	}
	if !slices.Equal(header, expectedHeader) {
		return fmt.Errorf("unexpected CSV header: got %q, want %q", header, expectedHeader)
	}

	batch := make([]*model.BIN, 0, batchSize)
	rowNumber := int64(1)
	for {
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		rowNumber++
		if readErr != nil {
			return fmt.Errorf("read CSV row %d: %w", rowNumber, readErr)
		}

		bin, parseErr := parseRow(row)
		if parseErr != nil {
			return fmt.Errorf("validate CSV row %d: %w", rowNumber, parseErr)
		}
		metadata.RowsRead++
		batch = append(batch, bin)
		if len(batch) == batchSize {
			if err := session.Stage(ctx, batch); err != nil {
				return fmt.Errorf("stage rows through %d: %w", rowNumber, err)
			}
			batch = batch[:0]
			fmt.Printf("\r  Validated and staged: %d", metadata.RowsRead)
		}
	}
	if metadata.RowsRead == 0 {
		return errors.New("dataset contains no records")
	}
	if len(batch) > 0 {
		if err := session.Stage(ctx, batch); err != nil {
			return fmt.Errorf("stage final batch: %w", err)
		}
	}
	result, err := session.Commit(ctx, metadata)
	if err != nil {
		return err
	}
	committed = true

	fmt.Printf(
		"\nDone! Imported %d records, deleted %d stale local records\nVersion: %s\nSHA-256: %s\n",
		result.RowsImported, result.RowsDeleted, metadata.SourceVersion, metadata.ChecksumSHA256,
	)
	return nil
}

func openDatasetFile(path string) (*os.File, error) {
	dataDir, err := filepath.Abs("data")
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	csvPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve CSV path: %w", err)
	}
	relPath, err := filepath.Rel(dataDir, csvPath)
	if err != nil || relPath == "." || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return nil, errors.New("CSV file must be located inside the project data directory")
	}
	dataRoot, err := os.OpenRoot(dataDir)
	if err != nil {
		return nil, fmt.Errorf("open data directory: %w", err)
	}
	file, err := dataRoot.Open(relPath)
	closeErr := dataRoot.Close()
	if err != nil {
		return nil, fmt.Errorf("open CSV: %w", err)
	}
	if closeErr != nil {
		_ = file.Close()
		return nil, fmt.Errorf("close data directory: %w", closeErr)
	}
	return file, nil
}

// parseRow maps a validated venelinkochev/bin-list-data CSV row to a BIN model.
func parseRow(row []string) (*model.BIN, error) {
	if len(row) != len(expectedHeader) {
		return nil, fmt.Errorf("expected %d columns, got %d", len(expectedHeader), len(row))
	}
	get := func(i int) string { return strings.TrimSpace(row[i]) }

	bin := get(0)
	if !validator.IsValidBIN(bin) {
		return nil, errors.New("BIN must be exactly 6 or 8 ASCII digits")
	}
	countryCode := strings.ToUpper(get(7))
	if countryCode != "" && !isASCIIAlpha(countryCode, 2) {
		return nil, errors.New("isoCode2 must be exactly 2 ASCII letters when provided")
	}
	fields := []struct {
		name  string
		value string
		max   int
	}{
		{"brand", get(1), 50},
		{"type", get(2), 20},
		{"category", get(3), 50},
		{"issuer", get(4), 200},
		{"issuer phone", get(5), 100},
		{"issuer URL", get(6), 200},
		{"country name", get(9), 100},
	}
	for _, field := range fields {
		if len(field.value) > field.max {
			return nil, fmt.Errorf("%s exceeds %d bytes", field.name, field.max)
		}
	}

	return &model.BIN{
		BIN:         bin,
		Brand:       strings.ToLower(get(1)),
		Type:        strings.ToLower(get(2)),
		Category:    strings.ToLower(get(3)),
		BankName:    get(4),
		BankPhone:   get(5),
		BankURL:     get(6),
		CountryCode: countryCode,
		CountryName: get(9),
		Source:      "local",
	}, nil
}

func isASCIIAlpha(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for i := range len(value) {
		if value[i] < 'A' || value[i] > 'Z' {
			return false
		}
	}
	return true
}
