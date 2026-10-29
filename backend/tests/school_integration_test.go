package tests

import (
	"encoding/json"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/school"
	"github.com/meddesk/meddesk/backend/internal/users"
	"testing"
)

func TestSchoolOwnershipIsolationAndWorkflow(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	admin, _ := managementAdmin(t, ctx, tx)
	owner := createFixture(t, ctx, tx)
	service := school.NewService(school.NewRepository(tx))
	if allowed, err := service.Access(ctx, admin.ID); err != nil || allowed {
		t.Fatalf("unconfigured school exposed: %v %v", allowed, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO school_workspace(owner_id) VALUES($1)`, owner.user); err != nil {
		t.Fatal(err)
	}
	// A clinic administrator has no implicit school access.
	if _, err := service.Assets(ctx, admin.ID, "", 20, 0); !errors.Is(err, school.ErrForbidden) {
		t.Fatalf("clinic admin read school: %v", err)
	}
	if _, err := service.SaveAsset(ctx, admin.ID, "", school.AssetInput{Name: "Camera", Kind: "CAMERA", Location: "Hall"}); !errors.Is(err, school.ErrForbidden) {
		t.Fatalf("clinic admin wrote school: %v", err)
	}
	password := "Synthetic-password-2026"
	if _, err := users.NewService(users.NewRepository(tx)).Update(ctx, admin, owner.user, users.UpdateInput{Email: owner.email, DisplayName: "Owner", Role: auth.RoleEmployee, DepartmentID: owner.department, IsActive: boolValue(true), Password: &password}); !errors.Is(err, users.ErrForbidden) {
		t.Fatalf("clinic admin could reset school owner's password: %v", err)
	}
	asset, err := service.SaveAsset(ctx, owner.user, "", school.AssetInput{Name: "Камера 1", Kind: "CAMERA", Location: "Дәліз", IPAddress: "192.168.10.20", InventoryNumber: "CAM-01"})
	if err != nil {
		t.Fatal(err)
	}
	var assetID string
	encoded, _ := json.Marshal(asset.ID)
	if err = json.Unmarshal(encoded, &assetID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateTicket(ctx, owner.user, school.TicketInput{Title: "Wrong device", Description: "Boundary test", Requester: "Test", Room: "101", Category: "NETWORK", Priority: "MEDIUM", AssetID: owner.article}); !errors.Is(err, school.ErrInvalid) {
		t.Fatalf("clinic UUID accepted as school device: %v", err)
	}
	if _, err := service.SaveAsset(ctx, owner.user, assetID, school.AssetInput{Name: "Camera", Kind: "CAMERA", Location: "Hall", Version: 99}); !errors.Is(err, school.ErrConflict) {
		t.Fatalf("stale asset edit accepted: %v", err)
	}
	filtered, err := service.Assets(ctx, owner.user, "Камера", 20, 0)
	if err != nil || filtered.Total != 1 {
		t.Fatalf("device search: %+v %v", filtered, err)
	}
	if _, err := service.Asset(ctx, admin.ID, assetID); !errors.Is(err, school.ErrForbidden) {
		t.Fatalf("clinic admin read device by ID: %v", err)
	}
	if _, err := service.Asset(ctx, owner.user, assetID); err != nil {
		t.Fatalf("device lookup: %v", err)
	}
	var clinicBefore int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tickets`).Scan(&clinicBefore); err != nil {
		t.Fatal(err)
	}
	ticket, err := service.CreateTicket(ctx, owner.user, school.TicketInput{Title: "Камера көрсетпейді", Description: "Дәліздегі камера ажыратылған", Requester: "Мұғалім", Room: "Дәліз", Category: "SURVEILLANCE", Priority: "HIGH", AssetID: assetID, LessonBlocked: true})
	if err != nil {
		t.Fatal(err)
	}
	var ticketID string
	encoded, _ = json.Marshal(ticket.ID)
	if err = json.Unmarshal(encoded, &ticketID); err != nil {
		t.Fatal(err)
	}
	page, err := service.Tickets(ctx, owner.user, "NEW", assetID, 20, 0)
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("history failed: %+v %v", page, err)
	}
	// Version checking and status workflow apply even to the workspace owner.
	if _, err = service.UpdateTicket(ctx, owner.user, ticketID, school.UpdateInput{Version: 1, Status: "RESOLVED", Priority: "HIGH", Resolution: "Done"}); !errors.Is(err, school.ErrInvalid) {
		t.Fatalf("skipped workflow: %v", err)
	}
	ticket, err = service.UpdateTicket(ctx, owner.user, ticketID, school.UpdateInput{Version: 1, Status: "IN_PROGRESS", Priority: "HIGH"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.UpdateTicket(ctx, owner.user, ticketID, school.UpdateInput{Version: 1, Status: "IN_PROGRESS", Priority: "LOW"}); !errors.Is(err, school.ErrConflict) {
		t.Fatalf("stale edit accepted: %v", err)
	}
	if _, err = service.UpdateTicket(ctx, owner.user, ticketID, school.UpdateInput{Version: ticket.Version, Status: "RESOLVED", Priority: "HIGH"}); !errors.Is(err, school.ErrInvalid) {
		t.Fatalf("empty resolution accepted: %v", err)
	}
	ticket, err = service.UpdateTicket(ctx, owner.user, ticketID, school.UpdateInput{Version: ticket.Version, Status: "RESOLVED", Priority: "HIGH", Resolution: "Кабель ауыстырылды"})
	if err != nil || !ticket.ResolvedAt.Valid {
		t.Fatalf("resolution failed: %v", err)
	}
	var clinicAfter int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tickets`).Scan(&clinicAfter); err != nil {
		t.Fatal(err)
	}
	if clinicBefore != clinicAfter {
		t.Fatal("school writes leaked into clinic tickets")
	}
	var history int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM school_history`).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if history != 4 {
		t.Fatalf("private history missing: %d", history)
	}
	// Transactional history failure must roll back the record as well.
	if _, err = tx.Exec(ctx, `CREATE FUNCTION pg_temp.reject_school_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test history unavailable'; END $$; CREATE TRIGGER reject_school_history BEFORE INSERT ON school_history FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_school_history()`); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveAsset(ctx, owner.user, "", school.AssetInput{Name: "Rollback", Kind: "ROUTER", Location: "Test"}); err == nil {
		t.Fatal("history failure accepted")
	}
	assets, err := service.Assets(ctx, owner.user, "", 20, 0)
	if err != nil || assets.Total != 1 {
		t.Fatalf("partial write committed: %+v %v", assets, err)
	}
	// Revoking ownership immediately blocks reads and writes on the next operation.
	if _, err = tx.Exec(ctx, `DELETE FROM school_workspace`); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Tickets(ctx, owner.user, "", "", 20, 0); !errors.Is(err, school.ErrForbidden) {
		t.Fatalf("revoked owner retained access: %v", err)
	}
}

func TestSchoolAccountAuditRollback(t *testing.T) {
	ctx, tx := databaseTransaction(t)
	owner := createFixture(t, ctx, tx)
	if _, err := tx.Exec(ctx, `INSERT INTO school_workspace(owner_id) VALUES($1)`, owner.user); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `CREATE FUNCTION pg_temp.reject_member_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test audit unavailable'; END $$; CREATE TRIGGER reject_member_audit BEFORE INSERT ON school_account_events FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_member_audit()`); err != nil {
		t.Fatal(err)
	}
	service := school.NewService(school.NewRepository(tx))
	if _, err := service.CreateMember(ctx, owner.user, school.MemberInput{Email: "rollback-" + owner.email, DisplayName: "Teacher", Password: "synthetic-password-2026"}); err == nil {
		t.Fatal("audit failure accepted")
	}
	members, err := service.Members(ctx, owner.user, 20, 0)
	if err != nil || members.Total != 0 {
		t.Fatalf("partial account creation: %+v %v", members, err)
	}
}
