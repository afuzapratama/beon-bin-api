-- Coordinate provider quotas across restarts and multiple API instances.

CREATE TABLE IF NOT EXISTS enrichment_usage (
    provider      VARCHAR(50) NOT NULL,
    period_start  DATE        NOT NULL,
    request_count BIGINT      NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider, period_start),
    CONSTRAINT enrichment_usage_request_count CHECK (request_count >= 0)
);
