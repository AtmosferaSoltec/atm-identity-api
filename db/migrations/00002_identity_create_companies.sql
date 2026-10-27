-- +goose Up
CREATE TABLE companies (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    document_number    CHAR(11)     NOT NULL UNIQUE CHECK (document_number ~ '^[0-9]{11}$'),
    legal_name         TEXT         NOT NULL,
    trade_name         TEXT,
    company_type       TEXT,
    taxpayer_status    TEXT,
    taxpayer_condition TEXT,
    email              TEXT,
    phone              TEXT,
    address            TEXT,
    district           TEXT,
    province           TEXT,
    department         TEXT,
    ubigeo             CHAR(6),
    field_sources      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    lookup_source      TEXT         NOT NULL CHECK (lookup_source IN ('apisperu', 'client')),
    looked_up_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE companies;
