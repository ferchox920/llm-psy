-- 0006_allow_multiple_profiles.up.sql

-- 1. Eliminar la restricción UNIQUE en user_id.
-- La restricción fue creada en 0001_init_schema.up.sql con el nombre predeterminado.
ALTER TABLE clone_profiles
    DROP CONSTRAINT clone_profiles_user_id_key;

-- 2. Crear un índice regular (no único) para búsquedas eficientes por user_id.
CREATE INDEX idx_clone_profiles_user_id ON clone_profiles(user_id);
