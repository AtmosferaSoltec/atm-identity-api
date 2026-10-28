-- +goose Up
-- Calls made to each upstream provider per calendar month (America/Lima), used
-- to stop before the provider's monthly quota is exhausted.
CREATE TABLE provider_usage (
    provider   TEXT        NOT NULL,
    month      DATE        NOT NULL,
    calls      INTEGER     NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, month)
);

-- Short-lived negative cache: documents the provider reported as nonexistent,
-- so asking again does not burn another upstream call.
CREATE TABLE lookup_misses (
    document_type   TEXT        NOT NULL CHECK (document_type IN ('dni', 'ruc')),
    document_number TEXT        NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (document_type, document_number)
);

-- Needed by the refresh cooldown: every row must know when it was last fetched.
UPDATE persons   SET looked_up_at = created_at WHERE looked_up_at IS NULL;
UPDATE companies SET looked_up_at = created_at WHERE looked_up_at IS NULL;
ALTER TABLE persons   ALTER COLUMN looked_up_at SET DEFAULT now(), ALTER COLUMN looked_up_at SET NOT NULL;
ALTER TABLE companies ALTER COLUMN looked_up_at SET DEFAULT now(), ALTER COLUMN looked_up_at SET NOT NULL;

-- +goose Down
ALTER TABLE companies ALTER COLUMN looked_up_at DROP NOT NULL, ALTER COLUMN looked_up_at DROP DEFAULT;
ALTER TABLE persons   ALTER COLUMN looked_up_at DROP NOT NULL, ALTER COLUMN looked_up_at DROP DEFAULT;
DROP TABLE lookup_misses;
DROP TABLE provider_usage;
