#!/usr/bin/env bash

set -euo pipefail

minimum="${COVERAGE_MINIMUM:-80}"
packages=(
  ./internal/handler
  ./internal/middleware
  ./internal/observability
  ./internal/requestcontext
  ./internal/service
  ./pkg/binlist
  ./pkg/cache
  ./pkg/handyapi
  ./pkg/validator
)

if [[ -n "${TEST_DATABASE_URL:-}" ]]; then
  packages+=(./internal/database ./internal/repository/postgres)
fi

failed=0
for package in "${packages[@]}"; do
  output="$(go test -cover "$package")"
  printf '%s\n' "$output"
  coverage="$(sed -nE 's/.*coverage: ([0-9]+([.][0-9]+)?)% of statements.*/\1/p' <<<"$output" | tail -n 1)"
  if [[ -z "$coverage" ]]; then
    printf 'could not determine coverage for %s\n' "$package" >&2
    failed=1
    continue
  fi
  if ! awk -v actual="$coverage" -v required="$minimum" 'BEGIN { exit !(actual >= required) }'; then
    printf '%s coverage %s%% is below required %s%%\n' "$package" "$coverage" "$minimum" >&2
    failed=1
  fi
done

exit "$failed"
