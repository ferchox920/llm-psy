ALTER TABLE users
    DROP COLUMN otp_expires_at,
    DROP COLUMN otp_code_hash,
    DROP COLUMN email_verified_at,
    DROP COLUMN password_hash,
    DROP COLUMN auth_subject,
    DROP COLUMN auth_provider;
