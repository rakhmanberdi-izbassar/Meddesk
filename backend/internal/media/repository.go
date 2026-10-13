package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
)

type Database interface {
	db.DBTX
	Begin(context.Context) (pgx.Tx, error)
}
type Repository struct {
	database Database
	queries  *db.Queries
}

func NewRepository(database Database) *Repository { return &Repository{database, db.New(database)} }
func (r *Repository) Create(ctx context.Context, actor auth.User, input File) (File, error) {
	uid, err := parseUUID(actor.ID)
	if err != nil {
		return File{}, err
	}
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return File{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	queries := db.New(tx)
	// Match administrative and knowledge mutation lock order. A role change
	// cannot race a file's metadata commit after the HTTP guard ran.
	if err := queries.LockUserManagement(ctx); err != nil {
		return File{}, err
	}
	current, err := queries.GetManagedUserForUpdate(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrForbidden
	}
	if err != nil {
		return File{}, err
	}
	if !current.IsActive || (current.Role != auth.RoleAdmin && current.Role != auth.RoleEngineer &&
		(current.Role != auth.RoleEmployee || input.Purpose != PurposeTicket)) {
		return File{}, ErrForbidden
	}
	stored, err := queries.MediaCreate(ctx, db.MediaCreateParams{UploaderID: uid, StorageKey: input.StorageKey, OriginalFilename: input.OriginalFilename, ContentType: input.ContentType, ByteCount: input.ByteCount, Sha256: input.SHA256, Purpose: input.Purpose})
	if err != nil {
		return File{}, err
	}
	metadata, err := json.Marshal(map[string]any{"purpose": stored.Purpose, "content_type": stored.ContentType, "byte_count": stored.ByteCount})
	if err != nil {
		return File{}, err
	}
	if err := queries.InsertManagementAudit(ctx, db.InsertManagementAuditParams{ActorID: uid, Action: "media.uploaded", EntityType: "media", EntityID: stored.ID, Metadata: metadata}); err != nil {
		return File{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		if errors.Is(err, pgx.ErrTxCommitRollback) {
			return File{}, err
		}
		return File{}, errors.Join(ErrCommitUncertain, err)
	}
	return fileDTO(stored), nil
}
func (r *Repository) Find(ctx context.Context, actor auth.User, id string) (File, error) {
	uid, err := parseUUID(actor.ID)
	if err != nil {
		return File{}, err
	}
	fid, err := parseUUID(id)
	if err != nil {
		return File{}, err
	}
	stored, err := r.queries.MediaAuthorizedGet(ctx, db.MediaAuthorizedGetParams{ActorID: uid, ID: fid})
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, err
	}
	return fileDTO(stored), nil
}
func parseUUID(text string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if !httpx.ValidUUID(text) || id.Scan(text) != nil || !id.Valid {
		return id, ErrInvalidInput
	}
	return id, nil
}
func uuidText(id pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", id.Bytes[:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:])
}
func fileDTO(stored db.MediaFile) File {
	id := uuidText(stored.ID)
	return File{ID: id, OriginalFilename: stored.OriginalFilename, ContentType: stored.ContentType, ByteCount: stored.ByteCount, Purpose: stored.Purpose, CreatedAt: stored.CreatedAt.Time, URL: "/api/v1/media/" + id, StorageKey: stored.StorageKey, SHA256: stored.Sha256}
}
