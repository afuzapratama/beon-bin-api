#!/usr/bin/env bash
# scripts/import_csv.sh
# Downloads the latest BIN dataset from venelinkochev/bin-list-data
# and imports it into the local database.

set -euo pipefail

DATA_DIR="$(cd "$(dirname "$0")/../data" && pwd)"
CSV_FILE="$DATA_DIR/bin-list-data.csv"

echo "==> Downloading BIN dataset..."
curl -fSL \
  "https://raw.githubusercontent.com/venelinkochev/bin-list-data/master/bin-list-data.csv" \
  -o "$CSV_FILE"

echo "==> Download complete: $CSV_FILE"
echo "==> Starting import..."

go run ./cmd/importer "$CSV_FILE"

echo "==> Import finished."
