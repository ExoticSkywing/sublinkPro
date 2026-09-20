package distribution

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

func ptr(value string) *string { return &value }

func TestCredentialDeletionRetainsAuditAndCardOwnership(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c, other := issueOne(t, s), issueOne(t, s)
	activate(t, s, c)
	activate(t, s, other)
	cards, err := s.CreateCards(ctx, CardInput{Name: "paid", Kind: "quarter", Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	redeemed, err := s.Redeem(ctx, c.Token, cards[0].Code, "api")
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.ByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordVisit(ctx, Visit{CredentialID: c.ID, Result: "allowed"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.DeleteCredentials(ctx, []uint{c.ID}); err != nil {
			t.Fatal(err)
		}
	}
	after, err := s.ByID(ctx, c.ID)
	if err != nil || after.Status != "revoked" || after.Token != "" {
		t.Fatal("deleted detail is not read-only metadata", err)
	}
	if !reflect.DeepEqual(before.Regions, after.Regions) || !before.ExpiresAt.Equal(*after.ExpiresAt) || !before.ActivatedAt.Equal(*after.ActivatedAt) || before.AccessCount != after.AccessCount {
		t.Fatal("deletion changed rights/cities/activation")
	}
	listed, err := s.Credentials(ctx, ListFilter{})
	if err != nil || listed.Total != 1 || listed.Items[0].ID != other.ID {
		t.Fatal("default list includes deleted link", err)
	}
	deleted, err := s.Credentials(ctx, ListFilter{Status: "revoked"})
	if err != nil || deleted.Total != 1 || deleted.Items[0].Token != "" {
		t.Fatal("deleted filter leaks token", err)
	}
	for _, patch := range []CredentialPatch{{Status: ptr("enabled")}, {Name: ptr("changed")}, {Rotate: true}} {
		if _, err := s.UpdateCredential(ctx, c.ID, patch); err != Inactive {
			t.Fatal("deleted link edited", err)
		}
	}
	for _, check := range []func() (Decision, error){
		func() (Decision, error) { return s.Check(ctx, c.Token, testCity(1)) },
		func() (Decision, error) { return s.CommitDelivery(ctx, c.Token, testCity(1), c.SubscriptionID) },
	} {
		d, err := check()
		if err != nil || d.Result != "revoked" {
			t.Fatal("deleted link authorized", err)
		}
	}
	if _, err := s.Redeem(ctx, c.Token, cards[0].Code, "api"); err != Inactive {
		t.Fatal("deleted link renewed", err)
	}
	if _, err := s.Redeem(ctx, other.Token, cards[0].Code, "api"); err != CardUsed {
		t.Fatal("paid ownership released", err)
	}
	history, err := s.Redemptions(ctx, ListFilter{CredentialID: c.ID})
	if err != nil || history.Total != 1 || history.Items[0].ID != redeemed.ID || !history.Items[0].CredentialDeleted || history.Items[0].CredentialName != c.Name {
		t.Fatal("redemption history changed", err)
	}
	visits, err := s.Visits(ctx, ListFilter{CredentialID: c.ID})
	if err != nil || visits.Total != 1 {
		t.Fatal("visits lost", err)
	}
	if _, err := s.Issue(ctx, IssueInput{Name: c.Name, Batch: c.Batch, Count: 1, SubscriptionID: c.SubscriptionID}); err != Conflict {
		t.Fatal("batch reused deleted link", err)
	}
}

func TestCredentialDeletionAtomicValidationAndLegacyRevocation(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c, other := issueOne(t, s), issueOne(t, s)
	for _, ids := range [][]uint{nil, {}, {0}, {c.ID, c.ID}, make([]uint, 101)} {
		if err := s.DeleteCredentials(ctx, ids); err != Invalid {
			t.Fatal("accepted invalid IDs", err)
		}
	}
	if err := s.DeleteCredentials(ctx, []uint{c.ID, 999999}); err != NotFound {
		t.Fatal(err)
	}
	row, _ := s.ByID(ctx, c.ID)
	if row.Status != "enabled" {
		t.Fatal("partial deletion")
	}
	if _, err := s.Credentials(ctx, ListFilter{Status: "invalid"}); err != Invalid {
		t.Fatal(err)
	}
	if _, err := s.UpdateCredential(ctx, other.ID, CredentialPatch{Status: ptr("revoked")}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCredentials(ctx, []uint{c.ID, other.ID}); err != nil {
		t.Fatal(err)
	}
	listed, err := s.Credentials(ctx, ListFilter{})
	if err != nil || listed.Total != 0 {
		t.Fatal("legacy/new deletion semantics differ", err)
	}
	d, err := s.CommitDelivery(ctx, c.Token, testCity(1), c.SubscriptionID)
	if err != nil || d.Result != "revoked" || d.Credential.ActivatedAt != nil {
		t.Fatal("deleted unactivated link activated", err)
	}
}

func TestCredentialDeletionSerializesWithUpdatesAndDelivery(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := issueOne(t, s)
	var wg sync.WaitGroup
	errors := make(chan error, 3)
	wg.Add(3)
	go func() { defer wg.Done(); errors <- s.DeleteCredentials(ctx, []uint{c.ID}) }()
	go func() {
		defer wg.Done()
		_, err := s.UpdateCredential(ctx, c.ID, CredentialPatch{Name: ptr("race")})
		if err == Inactive {
			err = nil
		}
		errors <- err
	}()
	go func() {
		defer wg.Done()
		_, err := s.CommitDelivery(ctx, c.Token, testCity(1), c.SubscriptionID)
		errors <- err
	}()
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	c, err := s.ByID(ctx, c.ID)
	if err != nil || c.Status != "revoked" || c.Token != "" {
		t.Fatal("concurrent write resurrected link", err)
	}
}
