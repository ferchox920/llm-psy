-- Habilita pgvector para embeddings
CREATE EXTENSION IF NOT EXISTS vector;

-- Personajes relevantes en la narrativa del clon
CREATE TABLE characters (
    id UUID PRIMARY KEY,
    clone_profile_id UUID NOT NULL REFERENCES clone_profiles(id) ON DELETE CASCADE,
    name VARCHAR NOT NULL,
    relation VARCHAR NOT NULL,
    archetype VARCHAR,
    bond_status VARCHAR,
    bond_level INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_characters_clone_name ON characters (clone_profile_id, name);

-- Memorias narrativas (episodicas) asociadas al clon
CREATE TABLE narrative_memories (
    id UUID PRIMARY KEY,
    clone_profile_id UUID NOT NULL REFERENCES clone_profiles(id) ON DELETE CASCADE,
    related_character_id UUID REFERENCES characters(id) ON DELETE SET NULL,
    content TEXT NOT NULL,
    embedding VECTOR(1536),
    importance INT NOT NULL DEFAULT 5,
    happened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Índice vectorial para búsquedas por similitud (coseno)
CREATE INDEX idx_narrative_memories_embedding ON narrative_memories USING ivfflat (embedding vector_cosine_ops);
CREATE INDEX idx_narrative_memories_character ON narrative_memories (related_character_id);
CREATE INDEX idx_narrative_memories_clone ON narrative_memories (clone_profile_id);
