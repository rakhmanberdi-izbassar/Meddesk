DROP INDEX IF EXISTS article_views_dedup_idx;
ALTER TABLE knowledge_articles DROP COLUMN version;
