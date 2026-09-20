package distribution

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestCardEditingAndGrantedBenefits(t *testing.T) {
	s, now := testStore(t)
	ctx := context.Background()
	rows, err := s.CreateCards(ctx, CardInput{Name: "mistake", Count: 1, Kind: "quarter"})
	if err != nil {
		t.Fatal(err)
	}
	card := rows[0]
	name, kind := "corrected", "year"
	end, _ := json.Marshal(now.Add(48 * time.Hour))
	if err := s.UpdateCard(ctx, card.ID, CardPatch{Name: &name, Kind: &kind, EndsAt: end}); err != nil {
		t.Fatal(err)
	}
	listed, err := s.Cards(ctx, ListFilter{})
	if err != nil || listed.Items[0].Kind != kind || listed.Items[0].Code != card.Code || listed.Items[0].Name != name {
		t.Fatal("edit changed code or failed", err)
	}
	if err := s.UpdateCard(ctx, card.ID, CardPatch{EndsAt: json.RawMessage("null")}); err != nil {
		t.Fatal(err)
	}
	owner := issueOne(t, s)
	activate(t, s, owner)
	r, err := s.Redeem(ctx, owner.Token, card.Code, "web")
	if err != nil || r.Kind != kind {
		t.Fatal("new benefits not applied", err)
	}
	before, _ := s.ByID(ctx, owner.ID)
	kind = "permanent"
	if err := s.UpdateCard(ctx, card.ID, CardPatch{Kind: &kind}); err != CardLocked {
		t.Fatal("redeemed type changed", err)
	}
	name = "renamed after redemption"
	if err := s.UpdateCard(ctx, card.ID, CardPatch{Name: &name, EndsAt: end}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCards(ctx, []uint{card.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCards(ctx, []uint{card.ID}); err != nil {
		t.Fatal("delete retry", err)
	}
	if err := s.SetCardEnabled(ctx, card.ID, true); err != NotFound {
		t.Fatal("revived deleted code", err)
	}
	if _, err := s.Redeem(ctx, owner.Token, card.Code, "api"); err != CardUnavailable {
		t.Fatal("deleted code accepted", err)
	}
	after, _ := s.ByID(ctx, owner.ID)
	if !after.ExpiresAt.Equal(*before.ExpiresAt) || after.Permanent != before.Permanent || after.Plan != before.Plan {
		t.Fatal("edit/delete changed granted benefits")
	}
	history, err := s.Redemptions(ctx, ListFilter{})
	if err != nil || history.Total != 1 || history.Items[0].ID != r.ID || history.Items[0].Kind != "year" {
		t.Fatal("redemption audit lost", err)
	}
	listed, err = s.Cards(ctx, ListFilter{})
	if err != nil || listed.Total != 0 {
		t.Fatal("deleted code leaked into listing", err)
	}
}

func TestFreeCardDeletionAndExplicitReissue(t *testing.T) {
	s, now := testStore(t)
	ctx := context.Background()
	owner, other := issueOne(t, s), issueOne(t, s)
	activate(t, s, owner)
	activate(t, s, other)
	*now = now.Add(16 * 24 * time.Hour)
	card, err := s.CurrentFreeCard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kind := "year"
	for _, patch := range []CardPatch{{Kind: &kind}, {EndsAt: json.RawMessage("null")}} {
		if err := s.UpdateCard(ctx, card.ID, patch); err != CardLocked {
			t.Fatal("free cycle modified", err)
		}
	}
	first, err := s.Redeem(ctx, owner.Token, card.Code, "api")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCards(ctx, []uint{card.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CurrentFreeCard(ctx); err != CardDeleted {
		t.Fatal("ordinary retrieval recreated code", err)
	}
	next, err := s.ReissueFreeCard(ctx)
	if err != nil || next.ID == card.ID || next.Code == card.Code || next.Cycle != card.Cycle || !next.EndsAt.Equal(*card.EndsAt) {
		t.Fatal("bad replacement", err)
	}
	if _, err := s.Redeem(ctx, other.Token, card.Code, "api"); err != CardUnavailable {
		t.Fatal("old code valid", err)
	}
	*now = now.Add(time.Hour)
	again, err := s.Redeem(ctx, owner.Token, next.Code, "api")
	if err != nil || again.ID != first.ID || !again.After.Equal(*first.After) {
		t.Fatal("reissue granted benefits twice", err)
	}
	if _, err := s.Redeem(ctx, other.Token, next.Code, "api"); err != nil {
		t.Fatal("new recipient cannot redeem", err)
	}
	if err := s.SetCardEnabled(ctx, next.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, get := range []func(context.Context) (Card, error){s.CurrentFreeCard, s.ReissueFreeCard} {
		same, err := get(ctx)
		if err != nil || same.ID != next.ID || same.Enabled {
			t.Fatal("disabled code replaced/re-enabled", err)
		}
	}
	if err := s.DeleteCards(ctx, []uint{next.ID}); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(7 * 24 * time.Hour)
	later, err := s.CurrentFreeCard(ctx)
	if err != nil || later.Cycle == next.Cycle {
		t.Fatal("next cycle blocked", err)
	}
}

func TestCardManagementValidationAndAtomicBatchDelete(t *testing.T) {
	s, now := testStore(t)
	ctx := context.Background()
	in := CardInput{Name: "batch", Batch: "retry-key", Count: 2, Kind: "quarter"}
	rows, err := s.CreateCards(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	blank, badKind := " ", "free"
	past, _ := json.Marshal(now.Add(-time.Hour))
	for _, patch := range []CardPatch{{}, {Name: &blank}, {Kind: &badKind}, {EndsAt: past}, {EndsAt: json.RawMessage(`"bad date"`)}} {
		if err := s.UpdateCard(ctx, rows[0].ID, patch); err != Invalid {
			t.Fatal("invalid patch accepted", err)
		}
	}
	if err := s.DeleteCards(ctx, []uint{rows[0].ID, 99999}); err != NotFound {
		t.Fatal(err)
	}
	listed, _ := s.Cards(ctx, ListFilter{})
	if listed.Total != 2 {
		t.Fatal("batch partially deleted")
	}
	for _, ids := range [][]uint{nil, {0}, {rows[0].ID, rows[0].ID}, make([]uint, 101)} {
		if err := s.DeleteCards(ctx, ids); err != Invalid {
			t.Fatal("bad IDs accepted", err)
		}
	}
	if err := s.SetCardEnabled(ctx, rows[0].ID, false); err != nil {
		t.Fatal(err)
	}
	listed, _ = s.Cards(ctx, ListFilter{Status: "disabled"})
	if listed.Total != 1 || listed.Items[0].ID != rows[0].ID {
		t.Fatal("status filter failed")
	}
	if err := s.DeleteCards(ctx, []uint{rows[0].ID, rows[1].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCards(ctx, in); err != Conflict {
		t.Fatal("batch retry resurrected codes", err)
	}
}

func TestCardManagementSerializesWithRedemption(t *testing.T) {
	for _, action := range []string{"edit", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, _ := testStore(t)
			ctx := context.Background()
			c := issueOne(t, s)
			activate(t, s, c)
			cards, err := s.CreateCards(ctx, CardInput{Name: "race", Count: 1, Kind: "quarter"})
			if err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			var editErr, redeemErr error
			var r Redemption
			wg.Add(2)
			go func() {
				defer wg.Done()
				if action == "delete" {
					editErr = s.DeleteCards(ctx, []uint{cards[0].ID})
				} else {
					kind := "year"
					editErr = s.UpdateCard(ctx, cards[0].ID, CardPatch{Kind: &kind})
				}
			}()
			go func() { defer wg.Done(); r, redeemErr = s.Redeem(ctx, c.Token, cards[0].Code, "api") }()
			wg.Wait()
			if action == "edit" {
				if redeemErr != nil || (editErr != nil && editErr != CardLocked) {
					t.Fatal(editErr, redeemErr)
				}
				if (editErr == nil && r.Kind != "year") || (editErr == CardLocked && r.Kind != "quarter") {
					t.Fatal("inconsistent edited rights")
				}
			} else {
				if editErr != nil || (redeemErr != nil && redeemErr != CardUnavailable) {
					t.Fatal(editErr, redeemErr)
				}
				if _, err := s.Redeem(ctx, c.Token, cards[0].Code, "api"); err != CardUnavailable {
					t.Fatal("deleted code usable", err)
				}
			}
		})
	}
}

func TestCardSoftDeleteMigrationPreservesExistingCode(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	rows, err := s.CreateCards(ctx, CardInput{Name: "legacy", Count: 1, Kind: "quarter"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Migrator().DropColumn(&Card{}, "deleted_at"); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	listed, err := s.Cards(ctx, ListFilter{})
	if err != nil || listed.Total != 1 || listed.Items[0].Code != rows[0].Code {
		t.Fatal("migration changed original", err)
	}
}
