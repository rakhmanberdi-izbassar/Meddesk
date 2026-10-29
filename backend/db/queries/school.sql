-- name: SchoolOwner :one
SELECT EXISTS(SELECT 1 FROM school_workspace w JOIN users u ON u.id=w.owner_id WHERE w.owner_id=sqlc.arg(actor_id) AND u.is_active) AS allowed;

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
INSERT INTO school_tickets(title,description,requester,room,category,priority,lesson_blocked,asset_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;

-- name: SchoolTicketForUpdate :one
SELECT * FROM school_tickets WHERE id=$1 FOR UPDATE;

-- name: SchoolUpdateTicket :one
UPDATE school_tickets SET status=$2,priority=$3,resolution=$4,version=version+1,resolved_at=CASE WHEN $2 IN ('RESOLVED','CLOSED') THEN COALESCE(resolved_at,now()) ELSE NULL END WHERE id=$1 AND version=sqlc.arg(expected_version) RETURNING *;

-- name: SchoolHistory :exec
INSERT INTO school_history(actor_id,entity_id,entity_type,action,version) VALUES($1,$2,$3,$4,$5);

-- name: SchoolAuthorize :one
SELECT w.owner_id FROM school_workspace w JOIN users u ON u.id=w.owner_id WHERE w.owner_id=$1 AND u.is_active FOR SHARE OF w,u;

-- name: IsPrivateWorkspaceOwner :one
SELECT EXISTS(SELECT 1 FROM school_workspace WHERE owner_id=$1) AS protected;

-- name: SchoolGetAsset :one
SELECT * FROM school_assets WHERE id=$1;
