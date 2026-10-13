-- name: AnalyticsTicketSummary :one
SELECT count(*) AS total,
       count(*) FILTER (WHERE status IN ('NEW', 'IN_PROGRESS', 'WAITING_FOR_USER')) AS open,
       count(*) FILTER (WHERE status IN ('RESOLVED', 'CLOSED')) AS resolved,
       count(*) FILTER (WHERE status = 'CLOSED') AS closed,
       COALESCE(avg(extract(epoch FROM (resolved_at - created_at))) FILTER (WHERE status IN ('RESOLVED', 'CLOSED')), 0)::float8 AS average_resolution_seconds
FROM tickets
WHERE created_at >= sqlc.arg(start_at)::timestamptz AND created_at < sqlc.arg(end_at)::timestamptz
 AND (sqlc.narg(department_id)::uuid IS NULL OR department_id = sqlc.narg(department_id))
 AND (sqlc.narg(category_id)::uuid IS NULL OR category_id = sqlc.narg(category_id))
 AND (sqlc.narg(assignee_id)::uuid IS NULL OR assignee_id = sqlc.narg(assignee_id));

-- name: AnalyticsDepartments :many
SELECT d.id, d.name, count(*) AS total,
 count(*) FILTER (WHERE t.status IN ('NEW','IN_PROGRESS','WAITING_FOR_USER')) AS open,
 count(*) FILTER (WHERE t.status IN ('RESOLVED','CLOSED')) AS resolved
FROM tickets t JOIN departments d ON d.id = t.department_id
WHERE t.created_at >= sqlc.arg(start_at)::timestamptz AND t.created_at < sqlc.arg(end_at)::timestamptz
 AND (sqlc.narg(department_id)::uuid IS NULL OR t.department_id = sqlc.narg(department_id))
 AND (sqlc.narg(category_id)::uuid IS NULL OR t.category_id = sqlc.narg(category_id))
 AND (sqlc.narg(assignee_id)::uuid IS NULL OR t.assignee_id = sqlc.narg(assignee_id))
GROUP BY d.id, d.name ORDER BY total DESC, d.name, d.id;

-- name: AnalyticsCategories :many
SELECT t.category_id AS id, COALESCE(c.name_kk, 'Санатсыз')::text AS name_kk, COALESCE(c.name_ru, 'Без категории')::text AS name_ru, count(*) AS total,
 count(*) FILTER (WHERE t.status IN ('NEW','IN_PROGRESS','WAITING_FOR_USER')) AS open,
 count(*) FILTER (WHERE t.status IN ('RESOLVED','CLOSED')) AS resolved
FROM tickets t LEFT JOIN knowledge_categories c ON c.id = t.category_id
WHERE t.created_at >= sqlc.arg(start_at)::timestamptz AND t.created_at < sqlc.arg(end_at)::timestamptz
 AND (sqlc.narg(department_id)::uuid IS NULL OR t.department_id = sqlc.narg(department_id))
 AND (sqlc.narg(category_id)::uuid IS NULL OR t.category_id = sqlc.narg(category_id))
 AND (sqlc.narg(assignee_id)::uuid IS NULL OR t.assignee_id = sqlc.narg(assignee_id))
GROUP BY t.category_id, c.name_kk, c.name_ru ORDER BY total DESC, name_kk, t.category_id NULLS LAST;

-- name: AnalyticsFeedbackSummary :one
SELECT count(*) AS attempts, count(*) FILTER (WHERE f.helpful) AS helpful
FROM article_feedback f JOIN users u ON u.id = f.user_id JOIN knowledge_articles a ON a.id = f.article_id
WHERE f.created_at >= sqlc.arg(start_at)::timestamptz AND f.created_at < sqlc.arg(end_at)::timestamptz
 AND (sqlc.narg(department_id)::uuid IS NULL OR u.department_id = sqlc.narg(department_id))
 AND (sqlc.narg(category_id)::uuid IS NULL OR a.category_id = sqlc.narg(category_id));

-- name: AnalyticsPopularArticles :many
WITH views AS (
 SELECT v.article_id, count(*) AS view_count FROM article_views v JOIN users u ON u.id = v.user_id JOIN knowledge_articles a ON a.id = v.article_id
 WHERE v.created_at >= sqlc.arg(start_at)::timestamptz AND v.created_at < sqlc.arg(end_at)::timestamptz
  AND (sqlc.narg(department_id)::uuid IS NULL OR u.department_id = sqlc.narg(department_id))
  AND (sqlc.narg(category_id)::uuid IS NULL OR a.category_id = sqlc.narg(category_id))
 GROUP BY v.article_id
), feedback AS (
 SELECT f.article_id, count(*) FILTER (WHERE f.helpful) AS helpful_count, count(*) FILTER (WHERE NOT f.helpful) AS not_helpful_count
 FROM article_feedback f JOIN users u ON u.id = f.user_id JOIN knowledge_articles a ON a.id = f.article_id
 WHERE f.created_at >= sqlc.arg(start_at)::timestamptz AND f.created_at < sqlc.arg(end_at)::timestamptz
  AND (sqlc.narg(department_id)::uuid IS NULL OR u.department_id = sqlc.narg(department_id))
  AND (sqlc.narg(category_id)::uuid IS NULL OR a.category_id = sqlc.narg(category_id))
 GROUP BY f.article_id
)
SELECT a.id, a.title, COALESCE(v.view_count, 0)::bigint AS view_count, COALESCE(f.helpful_count, 0)::bigint AS helpful_count, COALESCE(f.not_helpful_count, 0)::bigint AS not_helpful_count
FROM knowledge_articles a LEFT JOIN views v ON v.article_id = a.id LEFT JOIN feedback f ON f.article_id = a.id
WHERE v.article_id IS NOT NULL OR f.article_id IS NOT NULL
ORDER BY view_count DESC, helpful_count DESC, a.title, a.id LIMIT 10;

-- name: AnalyticsTicketTrend :many
SELECT to_char(created_at AT TIME ZONE sqlc.arg(timezone)::text, 'YYYY-MM-DD')::text AS day,
 count(*) AS created, count(*) FILTER (WHERE status IN ('RESOLVED','CLOSED')) AS resolved
FROM tickets
WHERE created_at >= sqlc.arg(start_at)::timestamptz AND created_at < sqlc.arg(end_at)::timestamptz
 AND (sqlc.narg(department_id)::uuid IS NULL OR department_id = sqlc.narg(department_id))
 AND (sqlc.narg(category_id)::uuid IS NULL OR category_id = sqlc.narg(category_id))
 AND (sqlc.narg(assignee_id)::uuid IS NULL OR assignee_id = sqlc.narg(assignee_id))
GROUP BY day ORDER BY day;

-- name: AnalyticsFeedbackTrend :many
SELECT to_char(f.created_at AT TIME ZONE sqlc.arg(timezone)::text, 'YYYY-MM-DD')::text AS day,
 count(*) AS attempts, count(*) FILTER (WHERE helpful) AS helpful
FROM article_feedback f JOIN users u ON u.id = f.user_id JOIN knowledge_articles a ON a.id = f.article_id
WHERE f.created_at >= sqlc.arg(start_at)::timestamptz AND f.created_at < sqlc.arg(end_at)::timestamptz
 AND (sqlc.narg(department_id)::uuid IS NULL OR u.department_id = sqlc.narg(department_id))
 AND (sqlc.narg(category_id)::uuid IS NULL OR a.category_id = sqlc.narg(category_id))
GROUP BY day ORDER BY day;

-- name: AnalyticsFilterDepartments :many
SELECT id, name FROM departments ORDER BY name, id;

-- name: AnalyticsFilterCategories :many
SELECT id, name_kk, name_ru FROM knowledge_categories ORDER BY name_kk, id;

-- name: AnalyticsFilterAssignees :many
SELECT DISTINCT u.id, u.display_name FROM users u JOIN roles r ON r.id = u.role_id
WHERE r.code IN ('IT_ENGINEER','ADMIN') OR EXISTS (SELECT 1 FROM tickets t WHERE t.assignee_id = u.id)
ORDER BY u.display_name, u.id;
