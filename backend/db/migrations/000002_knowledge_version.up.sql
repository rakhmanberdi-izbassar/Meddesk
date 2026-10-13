ALTER TABLE knowledge_articles ADD COLUMN version integer NOT NULL DEFAULT 1 CHECK (version > 0);
CREATE INDEX article_views_dedup_idx ON article_views (article_id, user_id, created_at DESC);
