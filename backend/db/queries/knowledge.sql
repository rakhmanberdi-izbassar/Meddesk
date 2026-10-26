-- name: KnowledgeListArticleIDs :many
SELECT a.id
FROM knowledge_articles a JOIN knowledge_categories c ON c.id=a.category_id
WHERE (NOT sqlc.arg(public_only)::boolean OR (a.is_published AND c.is_active))
 AND (sqlc.narg(published)::boolean IS NULL OR a.is_published=sqlc.narg(published))
 AND (sqlc.narg(category_id)::uuid IS NULL OR a.category_id=sqlc.narg(category_id))
 AND (sqlc.arg(tag)::text='' OR EXISTS(SELECT 1 FROM article_tags at JOIN knowledge_tags t ON t.id=at.tag_id WHERE at.article_id=a.id AND lower(t.name)=lower(sqlc.arg(tag))))
 AND (sqlc.arg(search)::text='' OR a.search_simple @@ plainto_tsquery('simple',sqlc.arg(search))
 OR a.search_russian @@ plainto_tsquery('russian',sqlc.arg(search))
 OR EXISTS(SELECT 1 FROM knowledge_article_translations tr WHERE tr.article_id=a.id AND (tr.search_russian @@ plainto_tsquery('russian',sqlc.arg(search)) OR strpos(lower(tr.title || ' ' || tr.description || ' ' || tr.symptoms || ' ' || tr.solution_steps::text),lower(sqlc.arg(search)))>0))
 OR strpos(lower(a.title || ' ' || a.description || ' ' || a.symptoms || ' ' || a.solution_steps::text),lower(sqlc.arg(search)))>0
 OR EXISTS(SELECT 1 FROM article_tags at JOIN knowledge_tags t ON t.id=at.tag_id WHERE at.article_id=a.id AND strpos(lower(t.name),lower(sqlc.arg(search)))>0)
 OR (length(sqlc.arg(search))>=3 AND word_similarity(lower(sqlc.arg(search)),lower(a.title))>=0.32))
ORDER BY CASE WHEN sqlc.arg(search)::text='' THEN 0 ELSE
 (CASE WHEN strpos(lower(a.title),lower(sqlc.arg(search)))>0 THEN 20 ELSE 0 END)
 + COALESCE((SELECT 5*ts_rank_cd(tr.search_russian,plainto_tsquery('russian',sqlc.arg(search))) + CASE WHEN strpos(lower(tr.title),lower(sqlc.arg(search)))>0 THEN 20 ELSE 0 END FROM knowledge_article_translations tr WHERE tr.article_id=a.id AND tr.locale='ru'),0)
 + 5*ts_rank_cd(a.search_simple,plainto_tsquery('simple',sqlc.arg(search)))
 + 5*ts_rank_cd(a.search_russian,plainto_tsquery('russian',sqlc.arg(search)))
 + word_similarity(lower(sqlc.arg(search)),lower(a.title)) END DESC,
 a.updated_at DESC,a.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: KnowledgeCountArticles :one
SELECT count(*)
FROM knowledge_articles a JOIN knowledge_categories c ON c.id=a.category_id
WHERE (NOT sqlc.arg(public_only)::boolean OR (a.is_published AND c.is_active))
 AND (sqlc.narg(published)::boolean IS NULL OR a.is_published=sqlc.narg(published))
 AND (sqlc.narg(category_id)::uuid IS NULL OR a.category_id=sqlc.narg(category_id))
 AND (sqlc.arg(tag)::text='' OR EXISTS(SELECT 1 FROM article_tags at JOIN knowledge_tags t ON t.id=at.tag_id WHERE at.article_id=a.id AND lower(t.name)=lower(sqlc.arg(tag))))
 AND (sqlc.arg(search)::text='' OR a.search_simple @@ plainto_tsquery('simple',sqlc.arg(search))
 OR a.search_russian @@ plainto_tsquery('russian',sqlc.arg(search))
 OR EXISTS(SELECT 1 FROM knowledge_article_translations tr WHERE tr.article_id=a.id AND (tr.search_russian @@ plainto_tsquery('russian',sqlc.arg(search)) OR strpos(lower(tr.title || ' ' || tr.description || ' ' || tr.symptoms || ' ' || tr.solution_steps::text),lower(sqlc.arg(search)))>0))
 OR strpos(lower(a.title || ' ' || a.description || ' ' || a.symptoms || ' ' || a.solution_steps::text),lower(sqlc.arg(search)))>0
 OR EXISTS(SELECT 1 FROM article_tags at JOIN knowledge_tags t ON t.id=at.tag_id WHERE at.article_id=a.id AND strpos(lower(t.name),lower(sqlc.arg(search)))>0)
 OR (length(sqlc.arg(search))>=3 AND word_similarity(lower(sqlc.arg(search)),lower(a.title))>=0.32));

-- name: KnowledgeArticleDetails :many
SELECT a.id,a.version,a.title,a.slug,a.description,a.category_id,
 COALESCE((SELECT jsonb_build_object('title',tr.title,'description',tr.description,'symptoms',tr.symptoms,'solution_steps',tr.solution_steps) FROM knowledge_article_translations tr WHERE tr.article_id=a.id AND tr.locale='ru'),'null'::jsonb)::jsonb AS translation_ru,
 c.name_kk AS category_name_kk,c.name_ru AS category_name_ru,c.is_active AS category_active,
 a.symptoms,a.solution_steps,a.author_id,u.display_name AS author_name,a.video_id,
 a.is_published,a.published_at,a.created_at,a.updated_at,
 COALESCE((SELECT array_agg(t.name ORDER BY t.name) FROM article_tags at JOIN knowledge_tags t ON t.id=at.tag_id WHERE at.article_id=a.id),ARRAY[]::text[])::text[] AS tags,
 COALESCE((SELECT array_agg(am.media_id ORDER BY am.position) FROM article_media am WHERE am.article_id=a.id),ARRAY[]::uuid[])::uuid[] AS image_ids,
 (SELECT count(*) FROM article_views av WHERE av.article_id=a.id) AS view_count,
 (SELECT count(*) FROM article_feedback af WHERE af.article_id=a.id AND af.helpful) AS helpful_count,
 (SELECT count(*) FROM article_feedback af WHERE af.article_id=a.id AND NOT af.helpful) AS not_helpful_count
FROM knowledge_articles a JOIN knowledge_categories c ON c.id=a.category_id JOIN users u ON u.id=a.author_id
WHERE a.id=ANY(sqlc.arg(ids)::uuid[])
 AND (NOT sqlc.arg(public_only)::boolean OR (a.is_published AND c.is_active))
ORDER BY array_position(sqlc.arg(ids)::uuid[],a.id);

-- name: KnowledgeFindSlug :one
SELECT a.id FROM knowledge_articles a JOIN knowledge_categories c ON c.id=a.category_id
WHERE a.slug=$1 AND (NOT sqlc.arg(public_only)::boolean OR (a.is_published AND c.is_active));

-- name: KnowledgeLockArticle :one
SELECT id FROM knowledge_articles WHERE id=$1 FOR UPDATE;

-- name: KnowledgeCreateArticle :one
INSERT INTO knowledge_articles(title,slug,description,category_id,symptoms,solution_steps,author_id,video_id,is_published,published_at)
VALUES(sqlc.arg(title),sqlc.arg(slug),sqlc.arg(description),sqlc.arg(category_id),sqlc.arg(symptoms),sqlc.arg(solution_steps),sqlc.arg(author_id),sqlc.narg(video_id),sqlc.arg(is_published),CASE WHEN sqlc.arg(is_published)::boolean THEN clock_timestamp() ELSE NULL END)
RETURNING id;

-- name: KnowledgeUpdateArticle :exec
UPDATE knowledge_articles SET title=sqlc.arg(title),slug=sqlc.arg(slug),description=sqlc.arg(description),category_id=sqlc.arg(category_id),symptoms=sqlc.arg(symptoms),solution_steps=sqlc.arg(solution_steps),video_id=sqlc.narg(video_id),
 is_published=sqlc.arg(is_published),published_at=CASE WHEN sqlc.arg(is_published)::boolean THEN COALESCE(published_at,clock_timestamp()) ELSE NULL END,version=version+1
WHERE id=sqlc.arg(id);

-- name: KnowledgeDeleteArticle :exec
DELETE FROM knowledge_articles WHERE id=$1;

-- name: KnowledgeArticleTicketLinks :one
SELECT EXISTS(SELECT 1 FROM ticket_articles WHERE article_id=$1);

-- name: KnowledgeRemoveArticleTags :exec
DELETE FROM article_tags WHERE article_id=$1;

-- name: KnowledgeEnsureTag :one
INSERT INTO knowledge_tags(name) VALUES($1) ON CONFLICT(lower(btrim(name))) DO UPDATE SET name=knowledge_tags.name RETURNING id;

-- name: KnowledgeLinkTag :exec
INSERT INTO article_tags(article_id,tag_id) VALUES($1,$2);

-- name: KnowledgeRemoveImages :exec
DELETE FROM article_media WHERE article_id=$1;

-- name: KnowledgeLinkImage :exec
INSERT INTO article_media(article_id,media_id,position) VALUES($1,$2,$3);

-- name: KnowledgeMediaForUpdate :one
SELECT id,uploader_id,purpose FROM media_files WHERE id=$1 FOR UPDATE;

-- name: KnowledgeListCategories :many
SELECT * FROM knowledge_categories WHERE (NOT sqlc.arg(public_only)::boolean OR is_active)
 AND (sqlc.narg(active)::boolean IS NULL OR is_active=sqlc.narg(active))
 AND (sqlc.arg(search)::text='' OR strpos(lower(name_kk || ' ' || name_ru || ' ' || description),lower(sqlc.arg(search)))>0)
ORDER BY name_kk,id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: KnowledgeCountCategories :one
SELECT count(*) FROM knowledge_categories WHERE (NOT sqlc.arg(public_only)::boolean OR is_active)
 AND (sqlc.narg(active)::boolean IS NULL OR is_active=sqlc.narg(active))
 AND (sqlc.arg(search)::text='' OR strpos(lower(name_kk || ' ' || name_ru || ' ' || description),lower(sqlc.arg(search)))>0);

-- name: KnowledgeGetCategory :one
SELECT * FROM knowledge_categories WHERE id=$1 FOR UPDATE;

-- name: KnowledgeCreateCategory :one
INSERT INTO knowledge_categories(name_kk,name_ru,slug,description,is_active) VALUES($1,$2,$3,$4,$5) RETURNING *;

-- name: KnowledgeUpdateCategory :one
UPDATE knowledge_categories SET name_kk=$2,name_ru=$3,slug=$4,description=$5,is_active=$6 WHERE id=$1 RETURNING *;

-- name: KnowledgeTags :many
SELECT t.name FROM knowledge_tags t WHERE (sqlc.arg(search)::text='' OR strpos(lower(t.name),lower(sqlc.arg(search)))>0)
 AND (NOT sqlc.arg(public_only)::boolean OR EXISTS(SELECT 1 FROM article_tags at JOIN knowledge_articles a ON a.id=at.article_id JOIN knowledge_categories c ON c.id=a.category_id WHERE at.tag_id=t.id AND a.is_published AND c.is_active))
ORDER BY t.name LIMIT sqlc.arg(page_limit);

-- name: KnowledgeRecordView :one
INSERT INTO article_views(article_id,user_id) SELECT $1,$2 WHERE NOT EXISTS(SELECT 1 FROM article_views WHERE article_id=$1 AND user_id=$2 AND created_at>clock_timestamp()-interval '15 minutes') RETURNING id;

-- name: KnowledgeFeedback :one
SELECT helpful FROM article_feedback WHERE article_id=$1 AND user_id=$2 AND attempt_id=$3;

-- name: KnowledgeInsertFeedback :exec
INSERT INTO article_feedback(article_id,user_id,attempt_id,helpful) VALUES($1,$2,$3,$4);

-- name: KnowledgeArticleVersion :one
SELECT version FROM knowledge_articles WHERE id=$1 FOR UPDATE;

-- name: KnowledgeMediaAttached :one
SELECT EXISTS(SELECT 1 FROM knowledge_articles WHERE id=$1 AND video_id=$2)
 OR EXISTS(SELECT 1 FROM article_media WHERE article_id=$1 AND media_id=$2);

-- name: KnowledgeUpsertRussianTranslation :exec
INSERT INTO knowledge_article_translations(article_id,locale,title,description,symptoms,solution_steps)
VALUES($1,'ru',$2,$3,$4,$5)
ON CONFLICT(article_id,locale) DO UPDATE SET title=EXCLUDED.title,description=EXCLUDED.description,symptoms=EXCLUDED.symptoms,solution_steps=EXCLUDED.solution_steps;
