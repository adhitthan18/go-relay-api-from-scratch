CREATE TABLE IF NOT EXISTS audit_log (
    id         VARCHAR(36)  NOT NULL PRIMARY KEY,
    terjadi_at DATETIME     NOT NULL,
    aksi       VARCHAR(32)  NOT NULL,
    user_id    VARCHAR(36)  NULL,
    device_id  VARCHAR(36)  NULL,
    detail     VARCHAR(255) NULL,
    ip         VARCHAR(45)  NULL,
    INDEX idx_audit_waktu (terjadi_at),
    INDEX idx_audit_device (device_id, terjadi_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci