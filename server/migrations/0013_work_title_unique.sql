CREATE UNIQUE INDEX IF NOT EXISTS idx_works_user_title
    ON works (tenant_id, user_id, title);
