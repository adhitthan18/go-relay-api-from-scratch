ALTER TABLE devices
    ADD COLUMN device_secret_hash VARCHAR(64) NULL AFTER hardware_id,
    ADD COLUMN status             VARCHAR(16) NOT NULL DEFAULT 'pending' AFTER device_secret_hash,
    ADD COLUMN firmware           VARCHAR(32) NULL,
    ADD COLUMN pairing_token_hash VARCHAR(64) NULL,
    ADD COLUMN pairing_expires_at DATETIME    NULL,
    ADD COLUMN claimed_at         DATETIME    NULL,
    ADD COLUMN claimed_by         VARCHAR(36) NULL,
    ADD UNIQUE KEY uq_pairing_token (pairing_token_hash),
    DROP COLUMN bootstrap_secret_hash,
    DROP COLUMN bootstrap_used_at