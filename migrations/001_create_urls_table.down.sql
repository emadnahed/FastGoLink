-- Drop indexes first
DROP INDEX IF EXISTS idx_urls_created_at;
DROP INDEX IF EXISTS idx_urls_expires_at;
DROP INDEX IF EXISTS idx_urls_short_code;

-- Drop the urls table
DROP TABLE IF EXISTS urls;
