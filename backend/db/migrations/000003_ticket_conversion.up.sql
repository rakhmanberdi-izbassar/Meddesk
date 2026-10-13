ALTER TABLE tickets ADD COLUMN converted_article_id uuid
    REFERENCES knowledge_articles(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX tickets_converted_article_idx ON tickets(converted_article_id)
    WHERE converted_article_id IS NOT NULL;
