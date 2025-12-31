ALTER TABLE users
    ADD COLUMN auth_provider TEXT,
    ADD COLUMN auth_subject TEXT,
    ADD COLUMN password_hash TEXT,
    ADD COLUMN email_verified_at TIMESTAMPTZ,
    ADD COLUMN otp_code_hash TEXT,
    ADD COLUMN otp_expires_at TIMESTAMPTZ;
