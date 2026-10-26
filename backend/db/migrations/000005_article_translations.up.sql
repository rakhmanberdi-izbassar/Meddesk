CREATE TABLE knowledge_article_translations (
    article_id uuid NOT NULL REFERENCES knowledge_articles(id) ON DELETE CASCADE,
    locale text NOT NULL CHECK (locale = 'ru'),
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 250),
    description text NOT NULL CHECK (length(btrim(description)) BETWEEN 1 AND 2000),
    symptoms text NOT NULL CHECK (length(btrim(symptoms)) BETWEEN 1 AND 10000),
    solution_steps jsonb NOT NULL CHECK (valid_solution_steps(solution_steps)),
    search_russian tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('russian'::regconfig, title), 'A') ||
        setweight(to_tsvector('russian'::regconfig, description), 'B') ||
        setweight(to_tsvector('russian'::regconfig, symptoms), 'C') ||
        setweight(jsonb_to_tsvector('russian'::regconfig, solution_steps, '["string"]'::jsonb), 'D')
    ) STORED,
    PRIMARY KEY (article_id, locale)
);
CREATE INDEX knowledge_article_translations_search_idx ON knowledge_article_translations USING gin(search_russian);
