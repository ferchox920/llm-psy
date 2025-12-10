-- 0007_add_emotional_weight_to_narrative.down.sql

-- Elimina la columna 'emotional_weight'.
ALTER TABLE narrative_memories
    DROP COLUMN emotional_weight;
