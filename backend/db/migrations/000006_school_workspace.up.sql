-- No automatic owner: clinic administrators must not inherit school access.
CREATE TABLE school_workspace (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 owner_id uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE RESTRICT
);
CREATE TABLE school_assets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 150),
 kind text NOT NULL CHECK(kind IN ('COMPUTER','PRINTER','CAMERA','RECORDER','ROUTER','SWITCH','PROJECTOR','OTHER')),
 location text NOT NULL CHECK(length(btrim(location)) BETWEEN 1 AND 200),
 model text NOT NULL DEFAULT '' CHECK(length(model)<=200),
 inventory_number text NOT NULL DEFAULT '' CHECK(length(inventory_number)<=100),
 ip_address inet,
 notes text NOT NULL DEFAULT '' CHECK(length(notes)<=3000),
 version integer NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX school_assets_inventory_idx ON school_assets(inventory_number) WHERE inventory_number<>'';
CREATE TABLE school_tickets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 number bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
 title text NOT NULL CHECK(length(btrim(title)) BETWEEN 1 AND 250),
 description text NOT NULL CHECK(length(btrim(description)) BETWEEN 1 AND 10000),
 requester text NOT NULL CHECK(length(btrim(requester)) BETWEEN 1 AND 150),
 room text NOT NULL CHECK(length(btrim(room)) BETWEEN 1 AND 200),
 category text NOT NULL CHECK(category IN ('COMPUTERS','PRINTING','NETWORK','SURVEILLANCE','CLASSROOM','ACCOUNTS','OTHER')),
 priority text NOT NULL CHECK(priority IN ('LOW','MEDIUM','HIGH','CRITICAL')),
 status text NOT NULL DEFAULT 'NEW' CHECK(status IN ('NEW','IN_PROGRESS','WAITING_FOR_USER','RESOLVED','CLOSED')),
 lesson_blocked boolean NOT NULL DEFAULT false,
 asset_id uuid REFERENCES school_assets(id) ON DELETE RESTRICT,
 resolution text NOT NULL DEFAULT '' CHECK(length(resolution)<=5000),
 version integer NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 resolved_at timestamptz,
 CHECK(status NOT IN ('RESOLVED','CLOSED') OR length(btrim(resolution))>0),
 CHECK((status IN ('RESOLVED','CLOSED')) = (resolved_at IS NOT NULL)),
 CHECK(resolved_at IS NULL OR resolved_at >= created_at)
);
CREATE INDEX school_tickets_status_idx ON school_tickets(status,created_at DESC);
CREATE INDEX school_tickets_asset_idx ON school_tickets(asset_id,created_at DESC);
-- Private history deliberately stays outside clinic audit exports.
CREATE TABLE school_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 actor_id uuid NOT NULL REFERENCES users(id),
 entity_id uuid NOT NULL,
 entity_type text NOT NULL CHECK(entity_type IN ('asset','ticket')),
 action text NOT NULL CHECK(action IN ('created','updated')),
 version integer NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER school_assets_updated_at BEFORE UPDATE ON school_assets FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER school_tickets_updated_at BEFORE UPDATE ON school_tickets FOR EACH ROW EXECUTE FUNCTION set_updated_at();
