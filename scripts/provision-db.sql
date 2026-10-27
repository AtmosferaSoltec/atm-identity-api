-- Provisions the dedicated role, database and schema for atm-identity-api.
-- Run once per Postgres instance (local container or the VPS), as superuser:
--
--   docker exec -i atmosfera-postgres psql -v ON_ERROR_STOP=1 -U postgres \
--     -v app_password="'<IDENTITY_DB_PASSWORD>'" -d postgres < scripts/provision-db.sql
--
-- Same confinement as the other Atmosfera projects: own login role, own
-- `identity` schema, no access to `public`.

CREATE ROLE identity_app WITH LOGIN PASSWORD :app_password;
CREATE DATABASE identity OWNER identity_app;
REVOKE CONNECT ON DATABASE identity FROM PUBLIC;
GRANT CONNECT ON DATABASE identity TO identity_app;

\connect identity

CREATE SCHEMA IF NOT EXISTS identity AUTHORIZATION identity_app;
ALTER SCHEMA public OWNER TO postgres;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM identity_app;
ALTER ROLE identity_app IN DATABASE identity SET search_path TO identity;
