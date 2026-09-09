-- Enforce valid BIN/IIN values, preserve unknown fields as NULL, and audit imports.

ALTER TABLE bins
    DROP CONSTRAINT IF EXISTS bins_bin_format;

ALTER TABLE bins
    ADD CONSTRAINT bins_bin_format
    CHECK (bin ~ '^([0-9]{6}|[0-9]{8})$');

ALTER TABLE bins
    ALTER COLUMN prepaid DROP DEFAULT,
    ALTER COLUMN country_latitude DROP DEFAULT,
    ALTER COLUMN country_longitude DROP DEFAULT;

-- The local CSV does not provide these fields. Previous defaults represented
-- unknown values as false/zero, so normalize existing local records to NULL.
UPDATE bins
SET prepaid = NULL,
    country_latitude = NULL,
    country_longitude = NULL
WHERE source = 'local';

CREATE TABLE IF NOT EXISTS dataset_imports (
    id               BIGSERIAL   PRIMARY KEY,
    source           VARCHAR(100) NOT NULL,
    source_url       TEXT         NOT NULL,
    source_version   VARCHAR(100) NOT NULL,
    checksum_sha256  CHAR(64),
    status           VARCHAR(20)  NOT NULL,
    rows_read        BIGINT       NOT NULL DEFAULT 0,
    rows_imported    BIGINT       NOT NULL DEFAULT 0,
    rows_deleted     BIGINT       NOT NULL DEFAULT 0,
    error_message    TEXT,
    started_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT dataset_imports_status
        CHECK (status IN ('succeeded', 'failed')),
    CONSTRAINT dataset_imports_checksum
        CHECK (checksum_sha256 IS NULL OR checksum_sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX IF NOT EXISTS idx_dataset_imports_completed_at
    ON dataset_imports(completed_at DESC);

CREATE INDEX IF NOT EXISTS idx_dataset_imports_source_version
    ON dataset_imports(source, source_version);
