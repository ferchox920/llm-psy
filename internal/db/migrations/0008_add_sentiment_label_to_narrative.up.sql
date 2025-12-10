-- 0008_add_sentiment_label_to_narrative.up.sql

-- Añade la columna 'sentiment_label' faltante.
ALTER TABLE narrative_memories
    ADD COLUMN sentiment_label VARCHAR(50) NOT NULL DEFAULT 'Neutral';
