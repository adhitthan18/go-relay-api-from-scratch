CREATE TABLE IF NOT EXISTS relay_commands (
    id            VARCHAR(36) NOT NULL PRIMARY KEY,
    device_id     VARCHAR(36) NOT NULL,
    desired_state VARCHAR(3)  NOT NULL,
    expires_at    DATETIME    NOT NULL,
    created_at    DATETIME    NOT NULL,
    fetched_at    DATETIME    NULL,
    acked_at      DATETIME    NULL,
    created_by    VARCHAR(36) NULL,
    INDEX idx_cmd_device_aktif (device_id, acked_at, expires_at),
    CONSTRAINT fk_cmd_device FOREIGN KEY (device_id)
        REFERENCES devices(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci