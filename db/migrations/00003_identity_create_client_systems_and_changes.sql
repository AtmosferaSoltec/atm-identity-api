-- +goose Up
-- One row per consuming system (hotel, iris, ...). The API key is stored hashed.
CREATE TABLE client_systems (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code         TEXT        NOT NULL UNIQUE CHECK (code ~ '^[a-z][a-z0-9_-]*$'),
    name         TEXT        NOT NULL,
    api_key_hash TEXT        NOT NULL UNIQUE,
    is_active    BOOLEAN     NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);

-- Audit trail of every enrichment (PATCH), for persons and companies.
CREATE TABLE record_changes (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entity_type      TEXT        NOT NULL CHECK (entity_type IN ('person', 'company')),
    entity_id        BIGINT      NOT NULL,
    client_system_id BIGINT      REFERENCES client_systems (id),
    changes          JSONB       NOT NULL, -- {"email": {"old": null, "new": "a@b.pe"}}
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX record_changes_entity_idx ON record_changes (entity_type, entity_id);

-- +goose Down
DROP TABLE record_changes;
DROP TABLE client_systems;
