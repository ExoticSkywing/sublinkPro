package distribution

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFreeRedemptionFeedbackAndCurrentRights(t *testing.T) {
	s, now := testStore(t)
	ctx := context.Background()
	c := issueOne(t, s)
	activate(t, s, c)
	*now = now.Add(16 * 24 * time.Hour)
	card, err := s.CurrentFreeCard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Redeem(ctx, c.Token, card.Code, "web")
	if err != nil || first.AlreadyRedeemed {
		t.Fatal("new redemption misclassified", err)
	}
	*now = now.Add(time.Hour)
	for i := 0; i < 2; i++ {
		again, err := s.Redeem(ctx, c.Token, card.Code, "web")
		if err != nil || !again.AlreadyRedeemed || again.ID != first.ID || !again.After.Equal(*first.After) {
			t.Fatal("retry not reported or extended benefits", err)
		}
	}
	// A historical redemption is not a statement that the subscription is still
	// valid: administrators may have corrected its expiry since then.
	past := now.Add(-time.Hour)
	if _, err := s.UpdateCredential(ctx, c.ID, CredentialPatch{ExpiresAt: &past}); err != nil {
		t.Fatal(err)
	}
	again, err := s.Redeem(ctx, c.Token, card.Code, "web")
	if err != nil || !again.AlreadyRedeemed {
		t.Fatal(err)
	}
	state, err := s.PublicStatus(ctx, c.Token)
	if err != nil || !state.ExpiresAt.Equal(past) {
		t.Fatal("retry overwrote actual expiry", err)
	}
	if err := s.DeleteCards(ctx, []uint{card.ID}); err != nil {
		t.Fatal(err)
	}
	replacement, err := s.ReissueFreeCard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	again, err = s.Redeem(ctx, c.Token, replacement.Code, "api")
	if err != nil || !again.AlreadyRedeemed || again.ID != first.ID {
		t.Fatal("replacement bypassed feedback/deduplication", err)
	}
	if s.DB.Migrator().HasColumn(&Redemption{}, "already_redeemed") {
		t.Fatal("request result was persisted")
	}
}

func TestConcurrentRedemptionReportsOnlyOneNewBenefit(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := issueOne(t, s)
	activate(t, s, c)
	cards, err := s.CreateCards(ctx, CardInput{Name: "feedback", Kind: "quarter", Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan Redemption, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.Redeem(ctx, c.Token, cards[0].Code, "api")
			if err != nil {
				t.Error(err)
				return
			}
			results <- r
		}()
	}
	wg.Wait()
	close(results)
	newCount, retries := 0, 0
	for r := range results {
		if r.AlreadyRedeemed {
			retries++
		} else {
			newCount++
		}
	}
	if newCount != 1 || retries != 5 {
		t.Fatalf("new %d, retries %d", newCount, retries)
	}
}

func TestRedemptionNamesAndDeletedCardDetails(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := issueOne(t, s)
	activate(t, s, c)
	cards, err := s.CreateCards(ctx, CardInput{Name: "Annual card", Kind: "year", Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Redeem(ctx, c.Token, cards[0].Code, "api")
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.ByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	name := "Readable recipient"
	updated, err := s.UpdateCredential(ctx, c.ID, CredentialPatch{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Token != before.Token || !updated.ExpiresAt.Equal(*before.ExpiresAt) || updated.Plan != before.Plan || updated.SubscriptionID != before.SubscriptionID {
		t.Fatal("name-only edit changed subscription rights or link")
	}
	cardName := "Readable card"
	if err := s.UpdateCard(ctx, cards[0].ID, CardPatch{Name: &cardName}); err != nil {
		t.Fatal(err)
	}
	detail, err := s.CardByID(ctx, cards[0].ID)
	if err != nil || detail.Deleted || detail.Code != cards[0].Code {
		t.Fatal("live detail unavailable", err)
	}
	if err := s.DeleteCards(ctx, []uint{cards[0].ID}); err != nil {
		t.Fatal(err)
	}
	page, err := s.Redemptions(ctx, ListFilter{CredentialID: c.ID})
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatal("history missing", err)
	}
	item := page.Items[0]
	if item.ID != r.ID || item.CredentialName != name || item.CardName != cardName || !item.CardDeleted {
		t.Fatal("incorrect joined names/deletion marker")
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), cards[0].Code) || strings.Contains(string(encoded), c.Token) || strings.Contains(string(encoded), "secret") {
		t.Fatal("history exposed secrets")
	}
	detail, err = s.CardByID(ctx, cards[0].ID)
	if err != nil || !detail.Deleted || detail.Name != cardName || detail.Code != "" {
		t.Fatal("deleted audit detail incorrect", err)
	}
	if _, err := s.CardByID(ctx, 0); err != Invalid {
		t.Fatal(err)
	}
	if _, err := s.CardByID(ctx, 9999); err != NotFound {
		t.Fatal(err)
	}
	// Legacy/manual database maintenance must not hide audit rows on LEFT JOIN.
	if err := s.DB.Unscoped().Delete(&Card{}, cards[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	page, err = s.Redemptions(ctx, ListFilter{})
	if err != nil || page.Total != 1 || page.Items[0].CardName != "" || page.Items[0].CredentialName != name {
		t.Fatal("missing relation removed history", err)
	}
}
