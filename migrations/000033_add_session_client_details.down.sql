DROP INDEX IF EXISTS idx_sessions_revoked_at;

DROP INDEX IF EXISTS idx_sessions_expires_at;

ALTER TABLE sessions
DROP CONSTRAINT IF EXISTS sessions_device_platform_check;

ALTER TABLE sessions
DROP COLUMN IF EXISTS language,
DROP COLUMN IF EXISTS timezone,
DROP COLUMN IF EXISTS screen,
DROP COLUMN IF EXISTS app_version,
DROP COLUMN IF EXISTS device_platform,
DROP COLUMN IF EXISTS device_name,
DROP COLUMN IF EXISTS device_id,
DROP COLUMN IF EXISTS device_model,
DROP COLUMN IF EXISTS device_brand,
DROP COLUMN IF EXISTS os_version,
DROP COLUMN IF EXISTS os_name,
DROP COLUMN IF EXISTS browser_version,
DROP COLUMN IF EXISTS browser_name,
DROP COLUMN IF EXISTS user_agent,
DROP COLUMN IF EXISTS last_seen_at,
DROP COLUMN IF EXISTS last_seen_ip,
DROP COLUMN IF EXISTS ip_address;
