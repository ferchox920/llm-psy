-- Traits para perfiles (soporta múltiples modelos psicológicos)
CREATE TABLE traits (
    id UUID PRIMARY KEY,
    profile_id UUID NOT NULL REFERENCES clone_profiles(id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    trait TEXT NOT NULL,
    value INT NOT NULL,
    confidence DECIMAL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (profile_id, category, trait)
);

CREATE INDEX idx_traits_profile_id ON traits(profile_id);
