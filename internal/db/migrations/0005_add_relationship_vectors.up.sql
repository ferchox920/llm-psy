ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS trust_level INT NOT NULL DEFAULT 50,
    ADD COLUMN IF NOT EXISTS intimacy_level INT NOT NULL DEFAULT 50,
    ADD COLUMN IF NOT EXISTS respect_level INT NOT NULL DEFAULT 50;

-- Si existia una columna previa relationship_level, migra parcialmente a intimidad
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'sessions' AND column_name = 'relationship_level'
    ) THEN
        EXECUTE 'UPDATE sessions SET intimacy_level = relationship_level WHERE intimacy_level = 50';
        -- Opcional: eliminar la columna vieja
        BEGIN
            ALTER TABLE sessions DROP COLUMN relationship_level;
        EXCEPTION WHEN undefined_column THEN
            -- ignorar si ya se elimino
            NULL;
        END;
    END IF;
END$$;
