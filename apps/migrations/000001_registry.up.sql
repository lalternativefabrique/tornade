CREATE TABLE IF NOT EXISTS registry_apps (
    name                 TEXT PRIMARY KEY,
    signing_key          BYTEA NOT NULL,
    app_key              BYTEA NOT NULL,
    signing_last4        TEXT NOT NULL,
    app_last4            TEXT NOT NULL,
    previous_signing_key BYTEA,
    previous_app_key     BYTEA,
    rotated_at           TIMESTAMPTZ,
    revoked_at           TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL
);
