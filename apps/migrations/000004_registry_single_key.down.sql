ALTER TABLE registry_apps
    ADD COLUMN signing_key          BYTEA,
    ADD COLUMN app_key              BYTEA,
    ADD COLUMN signing_last4        TEXT,
    ADD COLUMN app_last4            TEXT,
    ADD COLUMN previous_signing_key BYTEA,
    ADD COLUMN previous_app_key     BYTEA;

UPDATE registry_apps SET
    signing_key          = secret,
    app_key              = secret,
    signing_last4        = last4,
    app_last4            = last4,
    previous_signing_key = previous_secret,
    previous_app_key     = previous_secret;

ALTER TABLE registry_apps
    ALTER COLUMN signing_key   SET NOT NULL,
    ALTER COLUMN app_key       SET NOT NULL,
    ALTER COLUMN signing_last4 SET NOT NULL,
    ALTER COLUMN app_last4     SET NOT NULL,
    DROP COLUMN secret,
    DROP COLUMN last4,
    DROP COLUMN previous_secret;
