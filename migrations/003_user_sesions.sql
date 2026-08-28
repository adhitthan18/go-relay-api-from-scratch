CREATE TABLE IF NOT EXISTS user_sessions (
    token_hash VARCHAR(64) NOT NULL PRIMARY KEY,
    user_id    VARCHAR(36) NOT NULL,
    expires_at DATETIME    NOT NULL,
    created_at DATETIME    NOT NULL,
    CONSTRAINT fk_sessions_user FOREIGN KEY (user_id)
        REFERENCES app_users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci