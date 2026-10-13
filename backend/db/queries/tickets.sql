-- name: TicketActorForUpdate :one
SELECT u.id,u.email,u.display_name,r.code AS role,u.department_id,d.name AS department_name,u.is_active,d.is_active AS department_active
FROM users u JOIN roles r ON r.id=u.role_id JOIN departments d ON d.id=u.department_id WHERE u.id=$1 FOR UPDATE OF u;

-- name: TicketExportActor :one
SELECT r.code AS role, (u.is_active AND d.is_active)::boolean AS is_active
FROM users u JOIN roles r ON r.id=u.role_id JOIN departments d ON d.id=u.department_id WHERE u.id=$1;

-- name: TicketExportFilterLabels :one
SELECT COALESCE((SELECT name FROM departments WHERE id=sqlc.narg(department_id)::uuid), '')::text AS department_name,
COALESCE((SELECT name_kk FROM knowledge_categories WHERE id=sqlc.narg(category_id)::uuid), '')::text AS category_kk,
COALESCE((SELECT name_ru FROM knowledge_categories WHERE id=sqlc.narg(category_id)::uuid), '')::text AS category_ru,
COALESCE((SELECT display_name FROM users WHERE id=sqlc.narg(assignee_id)::uuid), '')::text AS assignee_name;

-- name: TicketList :many
SELECT t.id FROM tickets t JOIN users actor ON actor.id=sqlc.arg(actor_id) AND actor.is_active JOIN roles ar ON ar.id=actor.role_id
WHERE (ar.code IN ('ADMIN','IT_ENGINEER') OR t.requester_id=actor.id)
AND (NOT sqlc.arg(mine)::boolean OR t.requester_id=actor.id)
AND (sqlc.arg(search)::text='' OR strpos(lower(t.title||' '||t.description||' '||t.number::text),lower(sqlc.arg(search)))>0)
AND (sqlc.arg(status)::text='' OR t.status=sqlc.arg(status))
AND (sqlc.arg(priority)::text='' OR t.priority=sqlc.arg(priority))
AND (sqlc.narg(assignee_id)::uuid IS NULL OR t.assignee_id=sqlc.narg(assignee_id))
AND (sqlc.narg(department_id)::uuid IS NULL OR t.department_id=sqlc.narg(department_id))
AND (sqlc.narg(category_id)::uuid IS NULL OR t.category_id=sqlc.narg(category_id))
AND (sqlc.narg(start_at)::timestamptz IS NULL OR t.created_at>=sqlc.narg(start_at))
AND (sqlc.narg(end_at)::timestamptz IS NULL OR t.created_at<sqlc.narg(end_at))
ORDER BY t.created_at DESC,t.id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: TicketCount :one
SELECT count(*) FROM tickets t JOIN users actor ON actor.id=sqlc.arg(actor_id) AND actor.is_active JOIN roles ar ON ar.id=actor.role_id
WHERE (ar.code IN ('ADMIN','IT_ENGINEER') OR t.requester_id=actor.id)
AND (NOT sqlc.arg(mine)::boolean OR t.requester_id=actor.id)
AND (sqlc.arg(search)::text='' OR strpos(lower(t.title||' '||t.description||' '||t.number::text),lower(sqlc.arg(search)))>0)
AND (sqlc.arg(status)::text='' OR t.status=sqlc.arg(status))
AND (sqlc.arg(priority)::text='' OR t.priority=sqlc.arg(priority))
AND (sqlc.narg(assignee_id)::uuid IS NULL OR t.assignee_id=sqlc.narg(assignee_id))
AND (sqlc.narg(department_id)::uuid IS NULL OR t.department_id=sqlc.narg(department_id))
AND (sqlc.narg(category_id)::uuid IS NULL OR t.category_id=sqlc.narg(category_id))
AND (sqlc.narg(start_at)::timestamptz IS NULL OR t.created_at>=sqlc.narg(start_at))
AND (sqlc.narg(end_at)::timestamptz IS NULL OR t.created_at<sqlc.narg(end_at));

-- name: TicketExportRows :many
SELECT t.number, t.title, u.display_name AS requester_name, d.name AS department_name,
COALESCE(c.name_kk, 'Санатсыз')::text AS category_kk, COALESCE(c.name_ru, 'Без категории')::text AS category_ru,
t.priority, t.status, COALESCE(a.display_name, '')::text AS assignee_name, t.created_at, t.updated_at, t.resolved_at
FROM tickets t JOIN users u ON u.id=t.requester_id JOIN departments d ON d.id=t.department_id
LEFT JOIN knowledge_categories c ON c.id=t.category_id LEFT JOIN users a ON a.id=t.assignee_id
JOIN users actor ON actor.id=sqlc.arg(actor_id) AND actor.is_active JOIN roles ar ON ar.id=actor.role_id
WHERE ar.code IN ('ADMIN','IT_ENGINEER')
AND (NOT sqlc.arg(mine)::boolean OR t.requester_id=actor.id)
AND (sqlc.arg(search)::text='' OR strpos(lower(t.title||' '||t.description||' '||t.number::text),lower(sqlc.arg(search)))>0)
AND (sqlc.arg(status)::text='' OR t.status=sqlc.arg(status))
AND (sqlc.arg(priority)::text='' OR t.priority=sqlc.arg(priority))
AND (sqlc.narg(assignee_id)::uuid IS NULL OR t.assignee_id=sqlc.narg(assignee_id))
AND (sqlc.narg(department_id)::uuid IS NULL OR t.department_id=sqlc.narg(department_id))
AND (sqlc.narg(category_id)::uuid IS NULL OR t.category_id=sqlc.narg(category_id))
AND (sqlc.narg(start_at)::timestamptz IS NULL OR t.created_at>=sqlc.narg(start_at))
AND (sqlc.narg(end_at)::timestamptz IS NULL OR t.created_at<sqlc.narg(end_at))
ORDER BY t.created_at DESC,t.id LIMIT sqlc.arg(export_limit);

-- name: TicketDetails :one
SELECT t.*,u.display_name AS requester_name,d.name AS department_name,COALESCE(a.display_name,'')::text AS assignee_name,
ARRAY(SELECT ta.article_id FROM ticket_articles ta WHERE ta.ticket_id=t.id ORDER BY ta.article_id)::uuid[] AS article_ids,
ARRAY(SELECT ta.media_id FROM ticket_attachments ta WHERE ta.ticket_id=t.id ORDER BY ta.media_id)::uuid[] AS attachment_ids
FROM tickets t JOIN users u ON u.id=t.requester_id JOIN departments d ON d.id=t.department_id LEFT JOIN users a ON a.id=t.assignee_id
JOIN users actor ON actor.id=sqlc.arg(actor_id) AND actor.is_active JOIN roles ar ON ar.id=actor.role_id
WHERE t.id=sqlc.arg(id) AND (ar.code IN ('ADMIN','IT_ENGINEER') OR t.requester_id=actor.id);

-- name: TicketLock :one
SELECT * FROM tickets WHERE id=$1 FOR UPDATE;

-- name: TicketCreate :one
INSERT INTO tickets(title,description,requester_id,department_id,category_id,priority) VALUES($1,$2,$3,$4,$5,$6) RETURNING id;

-- name: TicketUpdate :exec
UPDATE tickets SET priority=$2,assignee_id=$3,status=$4,resolved_at=$5,version=version+1 WHERE id=$1;

-- name: TicketBump :exec
UPDATE tickets SET version=version+1 WHERE id=$1;

-- name: TicketLinkArticle :exec
INSERT INTO ticket_articles(ticket_id,article_id)VALUES($1,$2) ON CONFLICT DO NOTHING;

-- name: TicketLinkAttachment :exec
INSERT INTO ticket_attachments(ticket_id,media_id)VALUES($1,$2) ON CONFLICT DO NOTHING;

-- name: TicketCheckPublicArticle :one
SELECT a.id FROM knowledge_articles a JOIN knowledge_categories c ON c.id=a.category_id WHERE a.id=$1 AND a.is_published AND c.is_active FOR SHARE OF a,c;

-- name: TicketCheckMedia :one
SELECT id FROM media_files WHERE id=$1 AND purpose='TICKET_ATTACHMENT' AND (uploader_id=$2 OR $3::boolean) FOR SHARE;

-- name: TicketComments :many
SELECT c.id,c.author_id,u.display_name AS author_name,c.body,c.is_internal,c.created_at
FROM ticket_comments c JOIN users u ON u.id=c.author_id JOIN tickets t ON t.id=c.ticket_id
JOIN users actor ON actor.id=sqlc.arg(actor_id) AND actor.is_active JOIN roles ar ON ar.id=actor.role_id
WHERE c.ticket_id=sqlc.arg(ticket_id) AND (ar.code IN ('ADMIN','IT_ENGINEER') OR (t.requester_id=actor.id AND NOT c.is_internal)) ORDER BY c.created_at,c.id;

-- name: TicketCreateComment :one
INSERT INTO ticket_comments(ticket_id,author_id,body,is_internal)VALUES($1,$2,$3,$4) RETURNING id,author_id,body,is_internal,created_at;

-- name: TicketHistory :many
SELECT h.id,h.from_status,h.to_status,u.display_name AS actor_name,h.reason,h.created_at
FROM ticket_status_history h JOIN users u ON u.id=h.actor_id JOIN tickets t ON t.id=h.ticket_id
JOIN users actor ON actor.id=sqlc.arg(actor_id) AND actor.is_active JOIN roles ar ON ar.id=actor.role_id
WHERE h.ticket_id=sqlc.arg(ticket_id) AND (ar.code IN ('ADMIN','IT_ENGINEER') OR t.requester_id=actor.id) ORDER BY h.created_at,h.id;

-- name: TicketCreateHistory :exec
INSERT INTO ticket_status_history(ticket_id,from_status,to_status,actor_id,reason)VALUES($1,$2,$3,$4,$5);

-- name: TicketAssignees :many
SELECT u.id,u.display_name FROM users u JOIN roles r ON r.id=u.role_id JOIN departments d ON d.id=u.department_id
JOIN users actor ON actor.id=$1 AND actor.is_active JOIN roles ar ON ar.id=actor.role_id
WHERE u.is_active AND d.is_active AND r.code IN ('ADMIN','IT_ENGINEER') AND ar.code IN ('ADMIN','IT_ENGINEER') ORDER BY u.display_name,u.id;

-- name: TicketMarkConverted :exec
UPDATE tickets SET converted_article_id=$2,version=version+1 WHERE id=$1;

-- name: TicketDepartments :many
SELECT d.id,d.name FROM departments d
JOIN users actor ON actor.id=$1 AND actor.is_active JOIN roles ar ON ar.id=actor.role_id
WHERE ar.code IN ('ADMIN','IT_ENGINEER') ORDER BY d.name,d.id;
