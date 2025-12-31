-- Create urls table for storing shortened URLs
CREATE TABLE IF NOT EXISTS urls (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(10) UNIQUE NOT NULL,
    original_url TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    click_count BIGINT DEFAULT 0
);

-- Index for fast lookups by short_code
CREATE INDEX IF NOT EXISTS idx_urls_short_code ON urls(short_code);

-- Index for finding expired URLs (partial index for efficiency)
CREATE INDEX IF NOT EXISTS idx_urls_expires_at ON urls(expires_at) WHERE expires_at IS NOT NULL;

-- Index for analytics queries by creation date
CREATE INDEX IF NOT EXISTS idx_urls_created_at ON urls(created_at);
