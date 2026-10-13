-- name: MediaCreate :one
INSERT INTO media_files(uploader_id,storage_key,original_filename,content_type,byte_count,sha256,purpose)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;

-- name: MediaAuthorizedGet :one
SELECT m.* FROM media_files m
JOIN users u ON u.id=sqlc.arg(actor_id) AND u.is_active
JOIN roles r ON r.id=u.role_id
WHERE m.id=sqlc.arg(id) AND (
 m.uploader_id=u.id OR r.code='ADMIN'
 OR EXISTS(SELECT 1 FROM knowledge_articles a JOIN knowledge_categories c ON c.id=a.category_id
 WHERE (a.video_id=m.id OR EXISTS(SELECT 1 FROM article_media am WHERE am.article_id=a.id AND am.media_id=m.id))
 AND (r.code='IT_ENGINEER' OR (a.is_published AND c.is_active)))
 OR EXISTS(SELECT 1 FROM ticket_attachments ta JOIN tickets t ON t.id=ta.ticket_id
 WHERE ta.media_id=m.id AND (t.requester_id=u.id OR r.code='IT_ENGINEER'))
);
