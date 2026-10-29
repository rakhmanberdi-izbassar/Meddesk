-- name: SchoolOwner :one
SELECT EXISTS(SELECT 1 FROM users u CROSS JOIN school_workspace w WHERE u.id=sqlc.arg(actor_id) AND u.is_active AND (w.owner_id=u.id OR u.account_scope='SCHOOL')) AS allowed;

-- name: SchoolAssets :many
SELECT * FROM school_assets WHERE (sqlc.arg(search)::text='' OR strpos(lower(name || ' ' || location || ' ' || inventory_number),lower(sqlc.arg(search)))>0) ORDER BY created_at DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: SchoolAssetCount :one
SELECT count(*) FROM school_assets WHERE (sqlc.arg(search)::text='' OR strpos(lower(name || ' ' || location || ' ' || inventory_number),lower(sqlc.arg(search)))>0);

-- name: SchoolTickets :many
SELECT * FROM school_tickets WHERE (sqlc.arg(status)::text='' OR status=sqlc.arg(status)) AND (sqlc.arg(asset_filter)::text='' OR asset_id::text=sqlc.arg(asset_filter)) ORDER BY created_at DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: SchoolTicketCount :one
SELECT count(*) FROM school_tickets WHERE (sqlc.arg(status)::text='' OR status=sqlc.arg(status)) AND (sqlc.arg(asset_filter)::text='' OR asset_id::text=sqlc.arg(asset_filter));

-- name: SchoolCreateAsset :one
INSERT INTO school_assets(name,kind,location,model,inventory_number,ip_address,notes) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;

-- name: SchoolUpdateAsset :one
UPDATE school_assets SET name=$2,kind=$3,location=$4,model=$5,inventory_number=$6,ip_address=$7,notes=$8,version=version+1 WHERE id=$1 AND version=sqlc.arg(expected_version) RETURNING *;

-- name: SchoolCreateTicket :one
INSERT INTO school_tickets(title,description,requester,room,category,priority,lesson_blocked,asset_id,requester_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;

-- name: SchoolTicketForUpdate :one
SELECT * FROM school_tickets WHERE id=$1 FOR UPDATE;

-- name: SchoolUpdateTicket :one
UPDATE school_tickets SET status=$2,priority=$3,resolution=$4,public_reply=sqlc.arg(public_reply),version=version+1,resolved_at=CASE WHEN $2 IN ('RESOLVED','CLOSED') THEN COALESCE(resolved_at,now()) ELSE NULL END WHERE id=$1 AND version=sqlc.arg(expected_version) RETURNING *;

-- name: SchoolHistory :exec
INSERT INTO school_history(actor_id,entity_id,entity_type,action,version) VALUES($1,$2,$3,$4,$5);

-- name: SchoolAuthorize :one
SELECT w.owner_id FROM school_workspace w JOIN users u ON u.id=w.owner_id WHERE w.owner_id=$1 AND u.is_active AND u.account_scope='CLINIC' FOR SHARE OF w,u;

-- name: IsPrivateWorkspaceOwner :one
SELECT EXISTS(SELECT 1 FROM school_workspace WHERE owner_id=$1) AS protected;

-- name: SchoolGetAsset :one
SELECT * FROM school_assets WHERE id=$1;

-- name: SchoolMemberAuthorize :one
SELECT u.id,u.display_name FROM users u CROSS JOIN school_workspace w WHERE u.id=$1 AND u.account_scope='SCHOOL' AND u.is_active FOR SHARE OF u,w;

-- name: SchoolMembers :many
SELECT id,email,display_name,is_active,account_version AS version FROM users WHERE account_scope='SCHOOL' ORDER BY created_at DESC,id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: SchoolMemberCount :one
SELECT count(*) FROM users WHERE account_scope='SCHOOL';

-- name: SchoolCreateMember :one
INSERT INTO users(email,display_name,password_hash,role_id,department_id,account_scope)
VALUES($1,$2,$3,(SELECT id FROM roles WHERE code='EMPLOYEE'),NULL,'SCHOOL')
RETURNING id,email,display_name,is_active,account_version AS version;

-- name: SchoolUpdateMember :one
UPDATE users SET display_name=sqlc.arg(display_name),is_active=sqlc.arg(is_active),password_hash=COALESCE(sqlc.narg(password_hash)::text,password_hash),account_version=account_version+1
WHERE id=sqlc.arg(id) AND account_scope='SCHOOL' AND account_version=sqlc.arg(expected_version)
RETURNING id,email,display_name,is_active,account_version AS version;

-- name: SchoolAccountEvent :exec
INSERT INTO school_account_events(actor_id,user_id,action) VALUES($1,$2,$3);

-- name: SchoolMyTickets :many
SELECT id,number,title,description,room,category,priority,status,lesson_blocked,public_reply,created_at,updated_at,resolved_at
FROM school_tickets WHERE requester_id=sqlc.arg(actor_id) ORDER BY created_at DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: SchoolMyTicketCount :one
SELECT count(*) FROM school_tickets WHERE requester_id=$1;

-- name: SchoolMyTicket :one
SELECT id,number,title,description,room,category,priority,status,lesson_blocked,public_reply,created_at,updated_at,resolved_at
FROM school_tickets WHERE requester_id=sqlc.arg(actor_id) AND id=sqlc.arg(ticket_id);
