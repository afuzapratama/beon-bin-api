#!/usr/bin/env bash
# scripts/import_csv.sh
# Downloads the latest BIN dataset from venelinkochev/bin-list-data
# and imports it into the local database.

set -euo pipefail

DATA_DIR="$(cd "$(dirname "$0")/../data" && pwd)"
CSV_FILE="$DATA_DIR/bin-list-data.csv"
SOURCE_REPO="https://github.com/venelinkochev/bin-list-data.git"
TEMP_FILE="$(mktemp "$DATA_DIR/.bin-list-data.csv.XXXXXX")"
trap 'rm -f "$TEMP_FILE"' EXIT

SOURCE_VERSION="$(git ls-remote "$SOURCE_REPO" refs/heads/master | awk '{print $1}')"
if [[ ! "$SOURCE_VERSION" =~ ^[0-9a-f]{40}$ ]]; then
  echo "error: could not resolve an immutable upstream commit" >&2
  exit 1
fi

echo "==> Downloading BIN dataset at commit $SOURCE_VERSION..."
curl -fSL \
  "https://raw.githubusercontent.com/venelinkochev/bin-list-data/$SOURCE_VERSION/bin-list-data.csv" \
  -o "$TEMP_FILE"

echo "==> Download complete: $TEMP_FILE"
echo "==> Starting import..."

go run ./cmd/importer \
  -source-version "$SOURCE_VERSION" \
  -source-url "$SOURCE_REPO" \
  "$TEMP_FILE"

mv "$TEMP_FILE" "$CSV_FILE"

echo "==> Import finished."
