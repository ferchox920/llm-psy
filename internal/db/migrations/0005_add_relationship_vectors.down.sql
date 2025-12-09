ALTER TABLE sessions
    DROP COLUMN IF EXISTS trust_level,
    DROP COLUMN IF EXISTS intimacy_level,
    DROP COLUMN IF EXISTS respect_level;
