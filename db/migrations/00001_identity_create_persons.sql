-- +goose Up
CREATE TABLE persons (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    document_number    CHAR(8)      NOT NULL UNIQUE CHECK (document_number ~ '^[0-9]{8}$'),
    first_names        TEXT         NOT NULL,
    paternal_surname   TEXT         NOT NULL,
    maternal_surname   TEXT,
    verification_digit CHAR(1),
    birth_date         DATE,
    sex                TEXT         CHECK (sex IN ('male', 'female')),
    email              TEXT,
    phone              TEXT,
    address            TEXT,
    district           TEXT,
    province           TEXT,
    department         TEXT,
    ubigeo             CHAR(6),
    -- Which source last wrote each column, e.g. {"email": "system:hotel"}.
    field_sources      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    lookup_source      TEXT         NOT NULL CHECK (lookup_source IN ('apisperu', 'client')),
    looked_up_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE persons;
