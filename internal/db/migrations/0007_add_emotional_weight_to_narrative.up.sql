-- 0007_add_emotional_weight_to_narrative.up.sql

-- Añade la columna 'emotional_weight' faltante.
ALTER TABLE narrative_memories
    ADD COLUMN emotional_weight INT NOT NULL DEFAULT 1;
