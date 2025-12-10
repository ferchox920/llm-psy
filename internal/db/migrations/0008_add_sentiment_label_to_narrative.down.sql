-- 0008_add_sentiment_label_to_narrative.down.sql

-- Elimina la columna 'sentiment_label'.
ALTER TABLE narrative_memories
    DROP COLUMN sentiment_label;
