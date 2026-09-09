CREATE TABLE IF NOT EXISTS registry_apps (
    name                 TEXT PRIMARY KEY,
    signing_key          TEXT NOT NULL,
    app_key              TEXT NOT NULL,
    previous_signing_key TEXT,
    previous_app_key     TEXT,
    rotated_at           TIMESTAMPTZ,
    revoked_at           TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL
);
