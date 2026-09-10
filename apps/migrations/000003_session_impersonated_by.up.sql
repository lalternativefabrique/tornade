-- better-auth's admin plugin keeps the impersonating admin on the session;
-- 1.7.3 checks for the column at boot.
ALTER TABLE "session" ADD COLUMN IF NOT EXISTS "impersonatedBy" TEXT;
