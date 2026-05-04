-- migrations/001_create_bins.sql

CREATE TABLE IF NOT EXISTS bins (
    id               BIGSERIAL    PRIMARY KEY,
    bin              VARCHAR(8)   NOT NULL UNIQUE,
    brand            VARCHAR(50),
    type             VARCHAR(20),
    category         VARCHAR(50),
    bank_name        VARCHAR(200),
    bank_url         VARCHAR(200),
    bank_phone       VARCHAR(100),
    country_name     VARCHAR(100),
    country_code     VARCHAR(3),
    country_currency VARCHAR(10),
    country_latitude  DECIMAL(9,6) DEFAULT 0,
    country_longitude DECIMAL(9,6) DEFAULT 0,
    prepaid          BOOLEAN      DEFAULT FALSE,
    source           VARCHAR(50)  DEFAULT 'local',
    created_at       TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bins_bin        ON bins(bin);
CREATE INDEX IF NOT EXISTS idx_bins_brand      ON bins(brand);
CREATE INDEX IF NOT EXISTS idx_bins_country    ON bins(country_code);
CREATE INDEX IF NOT EXISTS idx_bins_type       ON bins(type);
