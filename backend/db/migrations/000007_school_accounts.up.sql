ALTER TABLE users ADD COLUMN account_scope text NOT NULL DEFAULT 'CLINIC' CHECK(account_scope IN ('CLINIC','SCHOOL'));
ALTER TABLE users ADD COLUMN account_version integer NOT NULL DEFAULT 1 CHECK(account_version>0);
ALTER TABLE users ALTER COLUMN department_id DROP NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_scope_department_check CHECK((account_scope='CLINIC' AND department_id IS NOT NULL) OR (account_scope='SCHOOL' AND department_id IS NULL));
CREATE INDEX users_school_accounts_idx ON users(created_at DESC) WHERE account_scope='SCHOOL';
ALTER TABLE school_tickets ADD COLUMN requester_id uuid REFERENCES users(id) ON DELETE RESTRICT;
ALTER TABLE school_tickets ADD COLUMN public_reply text NOT NULL DEFAULT '' CHECK(length(public_reply)<=5000);
CREATE INDEX school_tickets_requester_idx ON school_tickets(requester_id,created_at DESC);
CREATE TABLE school_account_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 action text NOT NULL CHECK(action IN ('created','updated','auth.login','auth.logout')),
 created_at timestamptz NOT NULL DEFAULT now()
);
