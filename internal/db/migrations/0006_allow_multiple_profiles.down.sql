-- 0006_allow_multiple_profiles.down.sql

-- 1. Eliminar el índice no único.
DROP INDEX idx_clone_profiles_user_id;

-- 2. Volver a añadir la restricción UNIQUE.
-- NOTA: Esto fallará si ya hay usuarios con múltiples perfiles.
ALTER TABLE clone_profiles
    ADD CONSTRAINT clone_profiles_user_id_key UNIQUE (user_id);
