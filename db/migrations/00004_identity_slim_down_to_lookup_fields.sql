-- +goose Up
-- The service only caches what APIs Peru returns: no client enrichment.
DROP TABLE record_changes;

ALTER TABLE persons
    DROP COLUMN verification_digit,
    DROP COLUMN birth_date,
    DROP COLUMN sex,
    DROP COLUMN email,
    DROP COLUMN phone,
    DROP COLUMN address,
    DROP COLUMN district,
    DROP COLUMN province,
    DROP COLUMN department,
    DROP COLUMN ubigeo,
    DROP COLUMN field_sources,
    DROP COLUMN lookup_source;

ALTER TABLE companies
    DROP COLUMN company_type,
    DROP COLUMN taxpayer_status,
    DROP COLUMN taxpayer_condition,
    DROP COLUMN email,
    DROP COLUMN phone,
    DROP COLUMN district,
    DROP COLUMN province,
    DROP COLUMN department,
    DROP COLUMN ubigeo,
    DROP COLUMN field_sources,
    DROP COLUMN lookup_source;

-- +goose Down
ALTER TABLE companies
    ADD COLUMN company_type       TEXT,
    ADD COLUMN taxpayer_status    TEXT,
    ADD COLUMN taxpayer_condition TEXT,
    ADD COLUMN email              TEXT,
    ADD COLUMN phone              TEXT,
    ADD COLUMN district           TEXT,
    ADD COLUMN province           TEXT,
    ADD COLUMN department         TEXT,
    ADD COLUMN ubigeo             CHAR(6),
    ADD COLUMN field_sources      JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN lookup_source      TEXT NOT NULL DEFAULT 'apisperu' CHECK (lookup_source IN ('apisperu', 'client'));

ALTER TABLE persons
    ADD COLUMN verification_digit CHAR(1),
    ADD COLUMN birth_date         DATE,
    ADD COLUMN sex                TEXT CHECK (sex IN ('male', 'female')),
    ADD COLUMN email              TEXT,
    ADD COLUMN phone              TEXT,
    ADD COLUMN address            TEXT,
    ADD COLUMN district           TEXT,
    ADD COLUMN province           TEXT,
    ADD COLUMN department         TEXT,
    ADD COLUMN ubigeo             CHAR(6),
    ADD COLUMN field_sources      JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN lookup_source      TEXT NOT NULL DEFAULT 'apisperu' CHECK (lookup_source IN ('apisperu', 'client'));

CREATE TABLE record_changes (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entity_type      TEXT        NOT NULL CHECK (entity_type IN ('person', 'company')),
    entity_id        BIGINT      NOT NULL,
    client_system_id BIGINT      REFERENCES client_systems (id),
    changes          JSONB       NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX record_changes_entity_idx ON record_changes (entity_type, entity_id);
