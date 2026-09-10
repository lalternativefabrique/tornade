-- Better Auth tables. Authentication is owned by the web app
-- (better-auth on TanStack Start via @lalternative/auth); the Go core only
-- verifies the JWT minted from the better-auth session. Schema is the canonical
-- one for better-auth ^1.6 with the admin plugin (role/banned columns).
--
-- Note: ids are TEXT (better-auth's id format). Domain tables that reference a
-- user (e.g. projects.owner_id) should use TEXT to match "user"(id).

CREATE TABLE IF NOT EXISTS "user" (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    "emailVerified" BOOLEAN NOT NULL DEFAULT false,
    image         TEXT,
    role          TEXT NOT NULL DEFAULT 'user',
    banned        BOOLEAN DEFAULT false,
    "banReason"   TEXT,
    "banExpires"  TIMESTAMPTZ,
    "createdAt"   TIMESTAMP NOT NULL DEFAULT NOW(),
    "updatedAt"   TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS "session" (
    id           TEXT PRIMARY KEY,
    "expiresAt"  TIMESTAMP NOT NULL,
    token        TEXT NOT NULL UNIQUE,
    "createdAt"  TIMESTAMP NOT NULL DEFAULT NOW(),
    "updatedAt"  TIMESTAMP NOT NULL DEFAULT NOW(),
    "ipAddress"  TEXT,
    "userAgent"  TEXT,
    "userId"     TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS session_userid_idx ON "session" ("userId");

CREATE TABLE IF NOT EXISTS "account" (
    id                       TEXT PRIMARY KEY,
    "accountId"              TEXT NOT NULL,
    "providerId"             TEXT NOT NULL,
    "userId"                 TEXT NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    "accessToken"            TEXT,
    "refreshToken"           TEXT,
    "idToken"                TEXT,
    "accessTokenExpiresAt"   TIMESTAMP,
    "refreshTokenExpiresAt"  TIMESTAMP,
    scope                    TEXT,
    password                 TEXT,
    "createdAt"              TIMESTAMP NOT NULL DEFAULT NOW(),
    "updatedAt"              TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS account_userid_idx ON "account" ("userId");

CREATE TABLE IF NOT EXISTS "verification" (
    id           TEXT PRIMARY KEY,
    identifier   TEXT NOT NULL,
    value        TEXT NOT NULL,
    "expiresAt"  TIMESTAMP NOT NULL,
    "createdAt"  TIMESTAMP NOT NULL DEFAULT NOW(),
    "updatedAt"  TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS verification_identifier_idx ON "verification" (identifier);
