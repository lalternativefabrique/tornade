ALTER TABLE registry_apps
    ADD COLUMN secret          BYTEA,
    ADD COLUMN last4           TEXT,
    ADD COLUMN previous_secret BYTEA;

UPDATE registry_apps SET
    secret          = app_key,
    last4           = app_last4,
    previous_secret = previous_app_key;

ALTER TABLE registry_apps
    ALTER COLUMN secret SET NOT NULL,
    ALTER COLUMN last4  SET NOT NULL,
    DROP COLUMN signing_key,
    DROP COLUMN app_key,
    DROP COLUMN signing_last4,
    DROP COLUMN app_last4,
    DROP COLUMN previous_signing_key,
    DROP COLUMN previous_app_key;
