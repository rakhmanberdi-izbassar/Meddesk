-- Refuse rollback while school identities exist: never turn them into clinic accounts.
DO $$ BEGIN IF EXISTS(SELECT 1 FROM users WHERE account_scope='SCHOOL') THEN RAISE EXCEPTION 'Remove school accounts safely before disposable rollback'; END IF; END $$;
DROP TABLE school_account_events;
DROP INDEX school_tickets_requester_idx;
ALTER TABLE school_tickets DROP COLUMN public_reply, DROP COLUMN requester_id;
ALTER TABLE users DROP CONSTRAINT users_scope_department_check;
ALTER TABLE users ALTER COLUMN department_id SET NOT NULL;
DROP INDEX users_school_accounts_idx;
ALTER TABLE users DROP COLUMN account_version, DROP COLUMN account_scope;
