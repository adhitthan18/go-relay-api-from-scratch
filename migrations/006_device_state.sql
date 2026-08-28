ALTER TABLE devices
    ADD COLUMN last_known_state VARCHAR(3) NULL AFTER display_name,
    ADD COLUMN last_seen_at     DATETIME   NULL AFTER last_known_state