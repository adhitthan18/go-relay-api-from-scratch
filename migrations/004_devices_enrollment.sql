ALTER TABLE devices
    ADD COLUMN bootstrap_secret_hash VARCHAR(64) NULL,
    ADD COLUMN bootstrap_used_at     DATETIME    NULL,
    ADD COLUMN device_token_hash     VARCHAR(64) NULL,
    ADD COLUMN enrolled_at           DATETIME    NULL,
    ADD COLUMN created_by            VARCHAR(36) NULL