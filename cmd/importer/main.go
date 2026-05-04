package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/beon/bin-api/internal/model"
	"github.com/beon/bin-api/internal/repository/postgres"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	_ = godotenv.Load()

	if len(os.Args) < 2 {
		log.Fatal("Usage: importer <path-to-binlist.csv>")
	}
	csvPath := os.Args[1]

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/bindb?sslmode=disable"
	}

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	repo := postgres.New(db)

	f, err := os.Open(csvPath)
	if err != nil {
		log.Fatalf("open csv: %v", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true

	if _, err := reader.Read(); err != nil {
		log.Fatalf("read header: %v", err)
	}

	var (
		batch   []*model.BIN
		total   int
		skipped int
	)

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("warn: skip row: %v", err)
			skipped++
			continue
		}

		b, err := parseRow(row)
		if err != nil {
			skipped++
			continue
		}

		batch = append(batch, b)
		if len(batch) >= 500 {
			if err := repo.BatchUpsert(batch); err != nil {
				log.Printf("error batch upsert: %v", err)
			}
			total += len(batch)
			batch = batch[:0]
			fmt.Printf("\r  Imported: %d", total)
		}
	}

	if len(batch) > 0 {
		if err := repo.BatchUpsert(batch); err != nil {
			log.Printf("error final batch upsert: %v", err)
		}
		total += len(batch)
	}

	fmt.Printf("\n\nDone! Imported %d records (%d skipped)\n", total, skipped)
}

// parseRow maps a CSV row to a BIN model.
// venelinkochev/bin-list-data column order (0-indexed):
// 0:BIN  1:Brand  2:Type  3:Category  4:Issuer(bank)
// 5:IssuerPhone  6:IssuerUrl  7:isoCode2  8:isoCode3  9:CountryName
func parseRow(row []string) (*model.BIN, error) {
	if len(row) < 8 {
		return nil, fmt.Errorf("not enough columns")
	}
	get := func(i int) string {
		if i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	return &model.BIN{
		BIN:         get(0),
		Brand:       strings.ToLower(get(1)),
		Type:        strings.ToLower(get(2)),
		Category:    strings.ToLower(get(3)),
		BankName:    get(4),
		BankPhone:   get(5),
		BankURL:     get(6),
		CountryCode: strings.ToUpper(get(7)),
		CountryName: get(9),
		Source:      "local",
	}, nil
}
