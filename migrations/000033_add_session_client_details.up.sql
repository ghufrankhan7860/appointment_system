ALTER TABLE sessions
ADD COLUMN ip_address INET NULL,
ADD COLUMN last_seen_ip INET NULL,
ADD COLUMN last_seen_at TIMESTAMPTZ NULL,
ADD COLUMN user_agent TEXT NOT NULL DEFAULT '',
ADD COLUMN browser_name VARCHAR(100) NOT NULL DEFAULT '',
ADD COLUMN browser_version VARCHAR(100) NOT NULL DEFAULT '',
ADD COLUMN os_name VARCHAR(100) NOT NULL DEFAULT '',
ADD COLUMN os_version VARCHAR(100) NOT NULL DEFAULT '',
ADD COLUMN device_brand VARCHAR(100) NOT NULL DEFAULT '',
ADD COLUMN device_model VARCHAR(100) NOT NULL DEFAULT '',
ADD COLUMN device_id VARCHAR(64) NOT NULL DEFAULT '',
ADD COLUMN device_name VARCHAR(100) NOT NULL DEFAULT '',
ADD COLUMN device_platform VARCHAR(10) NOT NULL DEFAULT '',
ADD COLUMN app_version VARCHAR(30) NOT NULL DEFAULT '',
ADD COLUMN screen VARCHAR(20) NOT NULL DEFAULT '',
ADD COLUMN timezone VARCHAR(64) NOT NULL DEFAULT '',
ADD COLUMN language VARCHAR(35) NOT NULL DEFAULT '';

UPDATE sessions
SET last_seen_at = updated_at;

ALTER TABLE sessions
ALTER COLUMN last_seen_at SET NOT NULL;

ALTER TABLE sessions
ADD CONSTRAINT sessions_device_platform_check CHECK (device_platform IN ('', 'web', 'android', 'ios'));

CREATE INDEX idx_sessions_expires_at
    ON sessions (expires_at);

CREATE INDEX idx_sessions_revoked_at
    ON sessions (revoked_at)
    WHERE revoked_at IS NOT NULL;
