CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = clock_timestamp();
    RETURN NEW;
END;
$$;

CREATE FUNCTION valid_solution_steps(steps jsonb) RETURNS boolean
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS $$
    SELECT CASE WHEN jsonb_typeof(steps) <> 'array' THEN false ELSE
        jsonb_array_length(steps) BETWEEN 1 AND 100
        AND NOT EXISTS (
            SELECT 1 FROM jsonb_array_elements(steps) step
            WHERE jsonb_typeof(step) <> 'string'
               OR length(btrim(step #>> '{}')) NOT BETWEEN 1 AND 5000
        ) END;
$$;

CREATE TABLE roles (
    id smallint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code text NOT NULL UNIQUE CHECK (code IN ('EMPLOYEE', 'IT_ENGINEER', 'ADMIN'))
);
INSERT INTO roles (code) VALUES ('EMPLOYEE'), ('IT_ENGINEER'), ('ADMIN');

CREATE TABLE departments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 150),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX departments_name_key ON departments (lower(btrim(name)));
CREATE TRIGGER departments_updated_at BEFORE UPDATE ON departments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL CHECK (length(email) BETWEEN 3 AND 254 AND email = btrim(email)
        AND email ~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$'),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 150),
    password_hash text NOT NULL CHECK (password_hash ~ '^\$2[aby]\$(0[4-9]|[12][0-9]|3[01])\$[./A-Za-z0-9]{53}$'),
    role_id smallint NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    department_id uuid NOT NULL REFERENCES departments(id) ON DELETE RESTRICT,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));
CREATE INDEX users_department_idx ON users (department_id);
CREATE INDEX users_role_idx ON users (role_id);
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token_hash bytea NOT NULL CHECK (octet_length(csrf_token_hash) = 32),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at AND last_seen_at >= created_at)
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expiry_idx ON sessions (expires_at);

CREATE TABLE knowledge_categories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug) <= 100),
    name_kk text NOT NULL CHECK (length(btrim(name_kk)) BETWEEN 1 AND 150),
    name_ru text NOT NULL CHECK (length(btrim(name_ru)) BETWEEN 1 AND 150),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER knowledge_categories_updated_at BEFORE UPDATE ON knowledge_categories
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE media_files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    uploader_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    storage_key text NOT NULL UNIQUE CHECK (storage_key ~ '^[a-f0-9]{32}\.(png|jpg|webp|mp4|webm)$'),
    original_filename text NOT NULL CHECK (length(original_filename) BETWEEN 1 AND 255
        AND original_filename !~ '[[:cntrl:]]'),
    content_type text NOT NULL,
    byte_count bigint NOT NULL CHECK (byte_count > 0),
    sha256 bytea NOT NULL CHECK (octet_length(sha256) = 32),
    purpose text NOT NULL CHECK (purpose IN ('ARTICLE_VIDEO', 'ARTICLE_IMAGE', 'TICKET_ATTACHMENT')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((purpose = 'ARTICLE_VIDEO' AND content_type IN ('video/mp4', 'video/webm')
            AND byte_count <= 524288000)
        OR (purpose IN ('ARTICLE_IMAGE', 'TICKET_ATTACHMENT')
            AND content_type IN ('image/png', 'image/jpeg', 'image/webp') AND byte_count <= 10485760))
);
CREATE INDEX media_files_uploader_idx ON media_files (uploader_id, created_at DESC);

CREATE TABLE knowledge_articles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug) <= 200),
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 250),
    description text NOT NULL CHECK (length(btrim(description)) BETWEEN 1 AND 2000),
    category_id uuid NOT NULL REFERENCES knowledge_categories(id) ON DELETE RESTRICT,
    symptoms text NOT NULL CHECK (length(btrim(symptoms)) BETWEEN 1 AND 10000),
    solution_steps jsonb NOT NULL CHECK (valid_solution_steps(solution_steps)),
    author_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    video_id uuid REFERENCES media_files(id) ON DELETE RESTRICT,
    is_published boolean NOT NULL DEFAULT false,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    search_simple tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple'::regconfig, title), 'A') ||
        setweight(to_tsvector('simple'::regconfig, description), 'B') ||
        setweight(to_tsvector('simple'::regconfig, symptoms), 'C') ||
        setweight(jsonb_to_tsvector('simple'::regconfig, solution_steps, '["string"]'::jsonb), 'D')
    ) STORED,
    search_russian tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('russian'::regconfig, title), 'A') ||
        setweight(to_tsvector('russian'::regconfig, description), 'B') ||
        setweight(to_tsvector('russian'::regconfig, symptoms), 'C') ||
        setweight(jsonb_to_tsvector('russian'::regconfig, solution_steps, '["string"]'::jsonb), 'D')
    ) STORED,
    CHECK ((is_published AND published_at IS NOT NULL AND published_at >= created_at)
        OR (NOT is_published AND published_at IS NULL))
);
CREATE INDEX knowledge_articles_category_idx ON knowledge_articles (category_id, is_published, updated_at DESC);
CREATE INDEX knowledge_articles_author_idx ON knowledge_articles (author_id);
CREATE INDEX knowledge_articles_video_idx ON knowledge_articles (video_id) WHERE video_id IS NOT NULL;
CREATE INDEX knowledge_articles_simple_idx ON knowledge_articles USING gin (search_simple);
CREATE INDEX knowledge_articles_russian_idx ON knowledge_articles USING gin (search_russian);
CREATE INDEX knowledge_articles_title_trgm_idx ON knowledge_articles USING gin (title gin_trgm_ops);
CREATE INDEX knowledge_articles_description_trgm_idx ON knowledge_articles USING gin (description gin_trgm_ops);
CREATE TRIGGER knowledge_articles_updated_at BEFORE UPDATE ON knowledge_articles
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE knowledge_tags (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 60),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX knowledge_tags_name_key ON knowledge_tags (lower(btrim(name)));
CREATE INDEX knowledge_tags_trgm_idx ON knowledge_tags USING gin (name gin_trgm_ops);

CREATE TABLE article_tags (
    article_id uuid NOT NULL REFERENCES knowledge_articles(id) ON DELETE CASCADE,
    tag_id uuid NOT NULL REFERENCES knowledge_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (article_id, tag_id)
);
CREATE INDEX article_tags_tag_idx ON article_tags (tag_id);

CREATE TABLE article_media (
    article_id uuid NOT NULL REFERENCES knowledge_articles(id) ON DELETE CASCADE,
    media_id uuid NOT NULL REFERENCES media_files(id) ON DELETE RESTRICT,
    position integer NOT NULL CHECK (position BETWEEN 0 AND 99),
    PRIMARY KEY (article_id, media_id),
    UNIQUE (article_id, position)
);
CREATE INDEX article_media_media_idx ON article_media (media_id);

CREATE TABLE article_feedback (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id uuid NOT NULL REFERENCES knowledge_articles(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    attempt_id uuid NOT NULL,
    helpful boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, article_id, attempt_id)
);
CREATE INDEX article_feedback_article_idx ON article_feedback (article_id, created_at DESC);
CREATE INDEX article_feedback_period_idx ON article_feedback (created_at DESC, helpful);

CREATE TABLE article_views (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    article_id uuid NOT NULL REFERENCES knowledge_articles(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX article_views_article_idx ON article_views (article_id, created_at DESC);
CREATE INDEX article_views_user_idx ON article_views (user_id);
CREATE INDEX article_views_period_idx ON article_views (created_at DESC);

CREATE TABLE tickets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 250),
    description text NOT NULL CHECK (length(btrim(description)) BETWEEN 1 AND 20000),
    requester_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    department_id uuid NOT NULL REFERENCES departments(id) ON DELETE RESTRICT,
    category_id uuid REFERENCES knowledge_categories(id) ON DELETE RESTRICT,
    priority text NOT NULL DEFAULT 'MEDIUM' CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    status text NOT NULL DEFAULT 'NEW' CHECK (status IN ('NEW', 'IN_PROGRESS', 'WAITING_FOR_USER', 'RESOLVED', 'CLOSED')),
    assignee_id uuid REFERENCES users(id) ON DELETE RESTRICT,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz,
    CHECK ((status IN ('RESOLVED', 'CLOSED') AND resolved_at IS NOT NULL AND resolved_at >= created_at)
        OR (status NOT IN ('RESOLVED', 'CLOSED') AND resolved_at IS NULL))
);
CREATE INDEX tickets_requester_idx ON tickets (requester_id, created_at DESC);
CREATE INDEX tickets_status_priority_idx ON tickets (status, priority, created_at DESC);
CREATE INDEX tickets_assignee_status_idx ON tickets (assignee_id, status);
CREATE INDEX tickets_department_idx ON tickets (department_id, created_at DESC);
CREATE INDEX tickets_category_idx ON tickets (category_id, created_at DESC);
CREATE TRIGGER tickets_updated_at BEFORE UPDATE ON tickets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE FUNCTION preserve_ticket_number() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.number IS DISTINCT FROM OLD.number THEN
        RAISE EXCEPTION 'ticket number is immutable' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER tickets_preserve_number BEFORE UPDATE ON tickets
    FOR EACH ROW EXECUTE FUNCTION preserve_ticket_number();

CREATE TABLE ticket_articles (
    ticket_id uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    article_id uuid NOT NULL REFERENCES knowledge_articles(id) ON DELETE RESTRICT,
    PRIMARY KEY (ticket_id, article_id)
);
CREATE INDEX ticket_articles_article_idx ON ticket_articles (article_id);

CREATE TABLE ticket_comments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    author_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    body text NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 10000),
    is_internal boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ticket_comments_ticket_idx ON ticket_comments (ticket_id, created_at);
CREATE INDEX ticket_comments_author_idx ON ticket_comments (author_id);
CREATE TRIGGER ticket_comments_updated_at BEFORE UPDATE ON ticket_comments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE ticket_attachments (
    ticket_id uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    media_id uuid NOT NULL REFERENCES media_files(id) ON DELETE RESTRICT,
    PRIMARY KEY (ticket_id, media_id)
);
CREATE INDEX ticket_attachments_media_idx ON ticket_attachments (media_id);

CREATE TABLE ticket_status_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ticket_id uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    from_status text CHECK (from_status IN ('NEW', 'IN_PROGRESS', 'WAITING_FOR_USER', 'RESOLVED', 'CLOSED')),
    to_status text NOT NULL CHECK (to_status IN ('NEW', 'IN_PROGRESS', 'WAITING_FOR_USER', 'RESOLVED', 'CLOSED')),
    actor_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reason text NOT NULL DEFAULT '' CHECK (length(reason) <= 2000),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (from_status IS DISTINCT FROM to_status)
);
CREATE INDEX ticket_status_history_ticket_idx ON ticket_status_history (ticket_id, created_at);
CREATE INDEX ticket_status_history_actor_idx ON ticket_status_history (actor_id);

CREATE TABLE audit_logs (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action text NOT NULL CHECK (length(btrim(action)) BETWEEN 1 AND 100),
    entity_type text NOT NULL CHECK (length(btrim(entity_type)) BETWEEN 1 AND 100),
    entity_id uuid,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_actor_idx ON audit_logs (actor_id, created_at DESC);
CREATE INDEX audit_logs_entity_idx ON audit_logs (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_logs_created_idx ON audit_logs (created_at DESC);
