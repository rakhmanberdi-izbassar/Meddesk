package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/media"
)

func mediaActor(t *testing.T, ctx context.Context, tx pgx.Tx, f fixture, role string) auth.User {
	t.Helper()
	var id string
	queryID(t, ctx, tx, &id, `INSERT INTO users(email,display_name,password_hash,role_id,department_id) SELECT gen_random_uuid()::text || '@example.invalid','Synthetic media actor',password_hash,(SELECT id FROM roles WHERE code=$2),department_id FROM users WHERE id=$1 RETURNING id::text`, f.user, role)
	return auth.User{ID: id, Role: role, IsActive: true, DepartmentID: f.department}
}
func mediaPNG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := png.Encode(&output, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
func mediaService(t *testing.T, tx pgx.Tx) (*media.Service, string) {
	t.Helper()
	directory := t.TempDir()
	files, err := media.NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { files.Close() })
	return media.NewService(media.NewRepository(tx), files, media.Options{}), directory
}
func uploadMediaImage(t *testing.T, ctx context.Context, service *media.Service, actor auth.User) media.File {
	t.Helper()
	file, err := service.Upload(ctx, actor, media.UploadInput{Purpose: media.PurposeImage, Filename: "synthetic-screen.png", ContentType: "image/png", Reader: bytes.NewReader(mediaPNG(t))})
	if err != nil {
		t.Fatal(err)
	}
	return file
}
func expectMedia(t *testing.T, ctx context.Context, service *media.Service, actor auth.User, id string, allowed bool) {
	t.Helper()
	_, file, err := service.Download(ctx, actor, id)
	if file != nil {
		defer file.Close()
	}
	if allowed {
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadAll(file); err != nil {
			t.Fatal(err)
		}
	} else if !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("private media visible to %s: %v", actor.Role, err)
	}
}

func TestMediaRepositoryPrivatePublishedAndFreshRoleAccess(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	f := createFixture(t, ctx, tx)
	owner := mediaActor(t, ctx, tx, f, auth.RoleEngineer)
	employee := mediaActor(t, ctx, tx, f, auth.RoleEmployee)
	staff := mediaActor(t, ctx, tx, f, auth.RoleEngineer)
	admin := mediaActor(t, ctx, tx, f, auth.RoleAdmin)
	service, directory := mediaService(t, tx)
	file := uploadMediaImage(t, ctx, service, owner)
	var audits int
	var digest []byte
	var key string
	if err := tx.QueryRow(ctx, `SELECT storage_key,sha256,(SELECT count(*) FROM audit_logs WHERE entity_id=m.id AND action='media.uploaded' AND entity_type='media') FROM media_files m WHERE m.id=$1`, file.ID).Scan(&key, &digest, &audits); err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256(mediaPNG(t))
	if audits != 1 || !bytes.Equal(expected[:], digest) || key != file.StorageKey {
		t.Fatal("metadata/hash/audit not persisted atomically")
	}
	expectMedia(t, ctx, service, owner, file.ID, true)
	expectMedia(t, ctx, service, admin, file.ID, true)
	expectMedia(t, ctx, service, employee, file.ID, false)
	expectMedia(t, ctx, service, staff, file.ID, false)
	execSQL(t, ctx, tx, `INSERT INTO article_media(article_id,media_id,position) VALUES($1,$2,0)`, f.article, file.ID)
	expectMedia(t, ctx, service, staff, file.ID, true)
	expectMedia(t, ctx, service, employee, file.ID, false)
	execSQL(t, ctx, tx, `UPDATE knowledge_articles SET is_published=true,published_at=clock_timestamp() WHERE id=$1`, f.article)
	expectMedia(t, ctx, service, employee, file.ID, true)
	execSQL(t, ctx, tx, `UPDATE knowledge_categories SET is_active=false WHERE id=$1`, f.category)
	expectMedia(t, ctx, service, employee, file.ID, false)
	expectMedia(t, ctx, service, staff, file.ID, true)
	execSQL(t, ctx, tx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='EMPLOYEE') WHERE id=$1`, staff.ID)
	expectMedia(t, ctx, service, staff, file.ID, false) // stale DTO cannot retain engineer access
	execSQL(t, ctx, tx, `UPDATE users SET is_active=false WHERE id=$1`, admin.ID)
	expectMedia(t, ctx, service, admin, file.ID, false)
	execSQL(t, ctx, tx, `UPDATE users SET role_id=(SELECT id FROM roles WHERE code='EMPLOYEE') WHERE id=$1`, owner.ID)
	_, err := service.Upload(ctx, owner, media.UploadInput{Purpose: media.PurposeImage, Filename: "second.png", Reader: bytes.NewReader(mediaPNG(t))})
	if !errors.Is(err, media.ErrForbidden) {
		t.Fatalf("stale engineer role committed upload: %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("forbidden metadata transaction leaked file: %v %v", entries, err)
	}
	expectMedia(t, ctx, service, owner, file.ID, true) // owner can still access their own upload
	execSQL(t, ctx, tx, `UPDATE users SET is_active=false WHERE id=$1`, owner.ID)
	expectMedia(t, ctx, service, owner, file.ID, false)
}
func TestMediaUploadAuditFailureRemovesFileAndRollsBackMetadata(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	f := createFixture(t, ctx, tx)
	actor := mediaActor(t, ctx, tx, f, auth.RoleEngineer)
	service, directory := mediaService(t, tx)
	execSQL(t, ctx, tx, `CREATE FUNCTION pg_temp.reject_media_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='media.uploaded' THEN RAISE EXCEPTION 'synthetic media audit failure'; END IF; RETURN NEW; END $$`)
	execSQL(t, ctx, tx, `CREATE TRIGGER reject_media_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_media_audit()`)
	_, err := service.Upload(ctx, actor, media.UploadInput{Purpose: media.PurposeImage, Filename: "synthetic-screen.png", Reader: bytes.NewReader(mediaPNG(t))})
	if err == nil {
		t.Fatal("audit failure ignored")
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM media_files WHERE uploader_id=$1`, actor.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || count != 0 || len(entries) != 0 {
		t.Fatalf("failed upload persisted count=%d files=%d err=%v", count, len(entries), err)
	}
}
