ALTER TABLE narrative_memories
    DROP COLUMN IF EXISTS emotional_intensity,
    DROP COLUMN IF EXISTS emotion_category;
