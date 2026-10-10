-- Server-checked session generation. Incrementing auth_version invalidates all
-- JWTs issued before a password/account assignment change.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS auth_version INTEGER NOT NULL DEFAULT 1;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_auth_version_positive_chk;

ALTER TABLE users
    ADD CONSTRAINT users_auth_version_positive_chk CHECK (auth_version > 0);

