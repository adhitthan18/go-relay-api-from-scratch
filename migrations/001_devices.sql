CREATE TABLE IF NOT EXISTS devices (
    id            VARCHAR(36)  NOT NULL PRIMARY KEY,
    hardware_id   VARCHAR(64)  NOT NULL UNIQUE,
    display_name  VARCHAR(120) NOT NULL,
    created_at    DATETIME     NOT NULL,
    updated_at    DATETIME     NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci