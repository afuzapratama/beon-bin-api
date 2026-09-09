package main

import (
	"strings"
	"testing"
)

func TestParseFlagsRequiresSourceVersion(t *testing.T) {
	if _, err := parseFlags([]string{"data/test.csv"}); err == nil {
		t.Fatal("parseFlags accepted an import without source version")
	}

	options, err := parseFlags([]string{"-source-version", "abc123", "data/test.csv"})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if options.sourceVersion != "abc123" || options.csvPath != "data/test.csv" {
		t.Fatalf("unexpected options: %+v", options)
	}
}

func TestParseRowValidatesSchemaAndValues(t *testing.T) {
	valid := []string{
		"411111", "VISA", "DEBIT", "CLASSIC", "Test Bank",
		"123", "https://bank.example", "id", "IDN", "Indonesia",
	}
	record, err := parseRow(valid)
	if err != nil {
		t.Fatalf("parseRow returned error: %v", err)
	}
	if record.BIN != "411111" || record.Brand != "visa" || record.CountryCode != "ID" {
		t.Fatalf("unexpected record: %+v", record)
	}
	if record.Prepaid != nil || record.CountryLatitude != nil || record.CountryLong != nil {
		t.Fatal("fields absent from CSV must remain unknown")
	}

	tests := []struct {
		name string
		row  []string
	}{
		{name: "wrong column count", row: valid[:9]},
		{name: "seven digit BIN", row: withValue(valid, 0, "4111111")},
		{name: "non ASCII BIN", row: withValue(valid, 0, "１２３４５６")},
		{name: "invalid country", row: withValue(valid, 7, "IDN")},
		{name: "oversized bank", row: withValue(valid, 4, strings.Repeat("x", 201))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseRow(test.row); err == nil {
				t.Fatal("parseRow unexpectedly accepted invalid row")
			}
		})
	}
}

func TestIsASCIIAlpha(t *testing.T) {
	if !isASCIIAlpha("ID", 2) {
		t.Fatal("valid ASCII country code was rejected")
	}
	for _, value := range []string{"id", "IDN", "I1", "ÎD"} {
		if isASCIIAlpha(value, 2) {
			t.Fatalf("invalid value %q was accepted", value)
		}
	}
}

func withValue(row []string, index int, value string) []string {
	clone := append([]string(nil), row...)
	clone[index] = value
	return clone
}
