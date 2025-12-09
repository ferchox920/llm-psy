-- Agrega carga emocional a las memorias narrativas
ALTER TABLE narrative_memories
    ADD COLUMN IF NOT EXISTS emotional_intensity INT NOT NULL DEFAULT 10,
    ADD COLUMN IF NOT EXISTS emotion_category VARCHAR(50),
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
