package distribution

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"sublink/internal/testutil"
	"sublink/models"
)

func TestExpiredNoticeSettingsValidationAndCompatibility(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	cfg, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	legacy := cfg.ExpiredMessage
	cfg.ExpiredMessages = []string{"  已到期  ", "请领取卡密续订", "{portal}", strings.Repeat("字", 200)}
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	want := []string{"已到期", "请领取卡密续订", "{portal}", strings.Repeat("字", 200)}
	stored, err := s.Settings(ctx)
	if err != nil || !reflect.DeepEqual(stored.ExpiredMessages, want) || stored.ExpiredMessage != legacy {
		t.Fatalf("settings roundtrip failed: %+v %v", stored, err)
	}
	// Legacy integrations must not erase the new list when writing old fields.
	cfg.ExpiredMessages = nil
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	for _, messages := range [][]string{{}, {" "}, {"line\nbreak"}, {"tab\tvalue"}, {"a\u2028b"}, {strings.Repeat("字", 201)}, make([]string, 11)} {
		cfg.ExpiredMessages = messages
		if err := s.SaveSettings(ctx, cfg); err != Invalid {
			t.Fatalf("invalid messages accepted: %q (%v)", messages, err)
		}
	}
	stored, err = s.Settings(ctx)
	if err != nil || !reflect.DeepEqual(stored.ExpiredMessages, want) {
		t.Fatal("legacy or invalid write erased notice list")
	}
	cfg.ExpiredMessages = []string{"只显示这一条"}
	cfg.ExpiredMessage = "" // New callers do not have to send the legacy field.
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	stored, err = s.Settings(ctx)
	if err != nil || !reflect.DeepEqual(stored.ExpiredMessages, cfg.ExpiredMessages) || stored.ExpiredMessage != legacy {
		t.Fatal("single notice or legacy preservation failed")
	}
}

func TestExpiredNoticeMigrationPreservesLegacySettings(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	cfg, _ := s.Settings(ctx)
	cfg.ExpiredMessage = "原来的到期提示"
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Migrator().DropColumn(&Settings{}, "expired_messages"); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Settings(ctx)
	if err != nil || stored.ExpiredMessage != cfg.ExpiredMessage || stored.ExpiredMessages != nil {
		t.Fatalf("migration changed legacy settings: %+v %v", stored, err)
	}
}

func testStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	db := testutil.OpenMemoryDB(t, "distribution")
	t.Cleanup(func() { testutil.CloseDB(t, db) })
	if err := db.AutoMigrate(&models.Subcription{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Subcription{ID: 1, Name: "resources"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	s := New(db, "test-encryption-key")
	s.Now = func() time.Time { return now }
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&Settings{}).Where("id = ?", 1).Update("cycle_anchor", now.Truncate(24*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	return s, &now
}
func issueOne(t *testing.T, s *Store) Credential {
	t.Helper()
	rows, err := s.Issue(context.Background(), IssueInput{Name: "Test", Count: 1, SubscriptionID: 1})
	if err != nil {
		t.Fatal(err)
	}
	return rows[0]
}
func testCity(n int) City {
	return City{Key: fmt.Sprintf("CN:%d", n), Country: "CN", Province: "Province", Name: fmt.Sprintf("City %d", n)}
}
func activate(t *testing.T, s *Store, c Credential) {
	t.Helper()
	d, err := s.CommitDelivery(context.Background(), c.Token, testCity(1), c.SubscriptionID)
	if err != nil || d.Result != "allowed" {
		t.Fatalf("activate: %+v %v", d, err)
	}
}

func TestActivationAndTwoCityLimitAreAtomic(t *testing.T) {
	s, _ := testStore(t)
	c := issueOne(t, s)
	ctx := context.Background()
	if _, err := s.Check(ctx, c.Token, testCity(1)); err != nil {
		t.Fatal(err)
	}
	stored, _ := s.Credential(ctx, c.Token)
	if stored.ActivatedAt != nil {
		t.Fatal("preflight activated trial")
	}
	var wg sync.WaitGroup
	results := make(chan string, 9)
	for i := 0; i < 9; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d, err := s.CommitDelivery(ctx, c.Token, testCity(i%3+1), c.SubscriptionID)
			if err != nil {
				results <- err.Error()
			} else {
				results <- d.Result
			}
		}(i)
	}
	wg.Wait()
	close(results)
	for result := range results {
		if result != "allowed" && result != "region_denied" {
			t.Errorf("unexpected result: %s", result)
		}
	}
	state, err := s.PublicStatus(ctx, c.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Regions) != 2 {
		t.Fatalf("regions = %d", len(state.Regions))
	}
	if state.ExpiresAt.Sub(*state.ActivatedAt) != 15*24*time.Hour {
		t.Fatal("trial granted more than once")
	}
}
func TestRevocationAndExpiryAreRecheckedBeforeDelivery(t *testing.T) {
	s, now := testStore(t)
	c := issueOne(t, s)
	activate(t, s, c)
	ctx := context.Background()
	*now = now.Add(16 * 24 * time.Hour)
	d, err := s.CommitDelivery(ctx, c.Token, testCity(2), c.SubscriptionID)
	if err != nil || d.Result != "expired" {
		t.Fatalf("expired: %+v %v", d, err)
	}
	status, _ := s.PublicStatus(ctx, c.Token)
	if len(status.Regions) != 1 {
		t.Fatal("expired request reserved second city")
	}
	value := "revoked"
	if _, err = s.UpdateCredential(ctx, c.ID, CredentialPatch{Status: &value}); err != nil {
		t.Fatal(err)
	}
	d, err = s.CommitDelivery(ctx, c.Token, testCity(1), c.SubscriptionID)
	if err != nil || d.Result != "revoked" {
		t.Fatalf("revoked: %+v %v", d, err)
	}
}
func TestPaidCardConcurrentRedemptionHasOneOwner(t *testing.T) {
	s, _ := testStore(t)
	a, b := issueOne(t, s), issueOne(t, s)
	activate(t, s, a)
	activate(t, s, b)
	ctx := context.Background()
	cards, err := s.CreateCards(ctx, CardInput{Name: "Quarter", Count: 1, Kind: "quarter"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, c := range []Credential{a, b} {
		wg.Add(1)
		go func(c Credential) {
			defer wg.Done()
			_, err := s.Redeem(ctx, c.Token, cards[0].Code, "web")
			results <- err
		}(c)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if err != CardUsed {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("owners = %d", success)
	}
	var redemption Redemption
	if err := s.DB.First(&redemption).Error; err != nil {
		t.Fatal(err)
	}
	owner := a
	if redemption.CredentialID == b.ID {
		owner = b
	}
	again, err := s.Redeem(ctx, owner.Token, cards[0].Code, "api")
	if err != nil || again.ID != redemption.ID {
		t.Fatal("retry changed redemption")
	}
	stored, _ := s.Credential(ctx, owner.Token)
	if !stored.ExpiresAt.Equal(*redemption.After) {
		t.Fatal("retry extended expiry")
	}
}
func TestFreeCyclesDoNotStackOrShortenPaidBenefits(t *testing.T) {
	s, now := testStore(t)
	ctx := context.Background()
	a, b := issueOne(t, s), issueOne(t, s)
	activate(t, s, a)
	activate(t, s, b)
	card, err := s.CurrentFreeCard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Redeem(ctx, a.Token, card.Code, "web"); err != Covered {
		t.Fatalf("trial must be protected: %v", err)
	}
	*now = now.Add(16 * 24 * time.Hour)
	card, err = s.CurrentFreeCard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	same, _ := s.CurrentFreeCard(ctx)
	if same.ID != card.ID || same.Code != card.Code {
		t.Fatal("rotation not idempotent")
	}
	r, err := s.Redeem(ctx, a.Token, card.Code, "web")
	if err != nil {
		t.Fatal(err)
	}
	if r.After.Sub(*now) != 7*24*time.Hour {
		t.Fatal("free days incorrect")
	}
	if _, err = s.Redeem(ctx, b.Token, card.Code, "web"); err != nil {
		t.Fatal("free card must support multiple credentials", err)
	}
	*now = now.Add(24 * time.Hour)
	again, err := s.Redeem(ctx, a.Token, card.Code, "web")
	if err != nil || again.ID != r.ID || !again.After.Equal(*r.After) {
		t.Fatal("duplicate free redemption extended time")
	}
	paid, err := s.CreateCards(ctx, CardInput{Name: "Year", Count: 1, Kind: "year"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Redeem(ctx, a.Token, paid[0].Code, "web"); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(7 * 24 * time.Hour)
	next, err := s.CurrentFreeCard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == card.ID {
		t.Fatal("new cycle did not rotate")
	}
	if _, err = s.Redeem(ctx, a.Token, next.Code, "web"); err != Covered {
		t.Fatal("free code consumed during paid period", err)
	}
}
func TestRegionApplicationCannotBorrowAnotherCredentialsVisit(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	a, b := issueOne(t, s), issueOne(t, s)
	activate(t, s, a)
	activate(t, s, b)
	if _, err := s.CommitDelivery(ctx, a.Token, testCity(2), a.SubscriptionID); err != nil {
		t.Fatal(err)
	}
	v := Visit{CredentialID: a.ID, Country: "CN", CityKey: testCity(3).Key, Province: "Province", City: "City 3", Result: "region_denied"}
	if err := s.RecordVisit(ctx, v); err != nil {
		t.Fatal(err)
	}
	var visit Visit
	s.DB.First(&visit)
	if _, err := s.ApplyRegion(ctx, ApplyInput{Link: b.Token, VisitID: visit.ID, Reason: "travel"}); err != Invalid {
		t.Fatal("cross-credential visit accepted", err)
	}
	r, err := s.ApplyRegion(ctx, ApplyInput{Link: a.Token, VisitID: visit.ID, Reason: "travel"})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.ApplyRegion(ctx, ApplyInput{Link: a.Token, VisitID: visit.ID, Reason: "travel again"})
	if err != nil || duplicate.ID != r.ID {
		t.Fatal("duplicate pending request")
	}
	if err = s.ReviewRegion(ctx, r.ID, ReviewInput{Approve: true}, "admin"); err != Invalid {
		t.Fatal("approval must specify replacement", err)
	}
	status, _ := s.PublicStatus(ctx, a.Token)
	if err = s.ReviewRegion(ctx, r.ID, ReviewInput{Approve: true, ReplaceRegionID: status.Regions[0].ID}, "admin"); err != nil {
		t.Fatal(err)
	}
	status, _ = s.PublicStatus(ctx, a.Token)
	if len(status.Regions) != 2 {
		t.Fatal("approval exceeded city cap")
	}
	if len(status.Candidates) != 0 {
		t.Fatal("approved city still offered for application")
	}
	d, err := s.Check(ctx, a.Token, testCity(3))
	if err != nil || d.Result != "allowed" {
		t.Fatal("approved city denied", err)
	}
}
func TestCalendarMonthsAndTokenRotation(t *testing.T) {
	jan := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	if got := addMonths(jan, 3); got.Month() != 4 || got.Day() != 30 {
		t.Fatal(got)
	}
	s, _ := testStore(t)
	c := issueOne(t, s)
	activate(t, s, c)
	updated, err := s.UpdateCredential(context.Background(), c.ID, CredentialPatch{Rotate: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Token == c.Token || updated.ActivatedAt == nil {
		t.Fatal("rotation lost identity")
	}
	if _, err = s.Credential(context.Background(), c.Token); err != NotFound {
		t.Fatal("old token still usable")
	}
	if updated.Secret == updated.Token {
		t.Fatal("secret stored in plaintext")
	}
}

func TestBatchNamesAndRetriesPreserveOriginalRecords(t *testing.T) {
	for _, count := range []int{1, 3, 10, 100} {
		t.Run(fmt.Sprintf("count_%d", count), func(t *testing.T) {
			s, _ := testStore(t)
			ctx := context.Background()
			links, err := s.Issue(ctx, IssueInput{Name: "9月", Batch: "names", Count: count, SubscriptionID: 1})
			if err != nil {
				t.Fatal(err)
			}
			cards, err := s.CreateCards(ctx, CardInput{Name: "9月", Batch: "names", Count: count, Kind: "quarter"})
			if err != nil {
				t.Fatal(err)
			}
			if len(links) != count || len(cards) != count {
				t.Fatal("unexpected batch size")
			}
			for i := range links {
				want := "9月"
				if count > 1 {
					want = fmt.Sprintf("9月-%03d", i+1)
				}
				if links[i].Name != want || cards[i].Name != want {
					t.Fatalf("item %d: link %q, card %q, want %q", i+1, links[i].Name, cards[i].Name, want)
				}
			}
			// Simulate a legacy name; retrying the batch must not rename it or rotate credentials.
			if err := s.DB.Model(&Credential{}).Where("id = ?", links[0].ID).Update("name", "9月 1").Error; err != nil {
				t.Fatal(err)
			}
			if err := s.DB.Model(&Card{}).Where("id = ?", cards[0].ID).Update("name", "9月 1").Error; err != nil {
				t.Fatal(err)
			}
			retriedLinks, err := s.Issue(ctx, IssueInput{Name: "Changed", Batch: "names", Count: count, SubscriptionID: 1})
			if err != nil {
				t.Fatal(err)
			}
			retriedCards, err := s.CreateCards(ctx, CardInput{Name: "Changed", Batch: "names", Count: count, Kind: "quarter"})
			if err != nil {
				t.Fatal(err)
			}
			if len(retriedLinks) != count || len(retriedCards) != count {
				t.Fatal("retry changed batch size")
			}
			for i := range links {
				want := links[i].Name
				if i == 0 {
					want = "9月 1"
				}
				if retriedLinks[i].Name != want || retriedLinks[i].ID != links[i].ID || retriedLinks[i].Token != links[i].Token {
					t.Fatalf("retry changed link %d", i+1)
				}
				if retriedCards[i].Name != want || retriedCards[i].ID != cards[i].ID || retriedCards[i].Code != cards[i].Code {
					t.Fatalf("retry changed card %d", i+1)
				}
			}
		})
	}
}

func TestResourceChangePreservesIdentityBenefitsAndRegions(t *testing.T) {
	for _, state := range []string{"unactivated", "trial", "permanent", "expired", "disabled"} {
		t.Run(state, func(t *testing.T) {
			s, now := testStore(t)
			ctx := context.Background()
			c := issueOne(t, s)
			if state != "unactivated" {
				activate(t, s, c)
			}
			if state == "permanent" {
				permanent := true
				if _, err := s.UpdateCredential(ctx, c.ID, CredentialPatch{Permanent: &permanent}); err != nil {
					t.Fatal(err)
				}
			}
			if state == "expired" {
				*now = now.Add(16 * 24 * time.Hour)
			}
			if state == "disabled" {
				status := "disabled"
				if _, err := s.UpdateCredential(ctx, c.ID, CredentialPatch{Status: &status}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := s.ByID(ctx, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.DB.Create(&models.Subcription{ID: 2, Name: "replacement"}).Error; err != nil {
				t.Fatal(err)
			}
			id := 2
			after, err := s.UpdateCredential(ctx, c.ID, CredentialPatch{SubscriptionID: &id})
			if err != nil || after.SubscriptionID != id {
				t.Fatalf("resource update failed: %v", err)
			}
			after.SubscriptionID = before.SubscriptionID
			after.UpdatedAt = before.UpdatedAt
			if !reflect.DeepEqual(before, after) {
				t.Fatal("resource change modified identity, rights, regions or history")
			}
		})
	}
}

func TestResourceChangeValidationAndInFlightDelivery(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := issueOne(t, s)
	if err := s.DB.Create(&models.Subcription{ID: 2, Name: "replacement"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Create(&models.Subcription{ID: 3, Name: "deleted"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Delete(&models.Subcription{}, 3).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{0, -1, 3, 999} {
		name := "must not persist"
		if _, err := s.UpdateCredential(ctx, c.ID, CredentialPatch{Name: &name, SubscriptionID: &id}); err != Invalid {
			t.Fatalf("invalid resource %d: %v", id, err)
		}
	}
	before, err := s.ByID(ctx, c.ID)
	if err != nil || before.Name != c.Name || before.SubscriptionID != c.SubscriptionID {
		t.Fatal("invalid update was not atomic")
	}
	id := 2
	if _, err := s.UpdateCredential(ctx, c.ID, CredentialPatch{SubscriptionID: &id}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitDelivery(ctx, c.Token, testCity(1), c.SubscriptionID); err != Conflict {
		t.Fatalf("old rendered resources authorized: %v", err)
	}
	stored, err := s.ByID(ctx, c.ID)
	if err != nil || stored.ActivatedAt != nil || stored.AccessCount != 0 || len(stored.Regions) != 0 {
		t.Fatal("discarded old resources activated the credential")
	}
	if d, err := s.CommitDelivery(ctx, c.Token, testCity(1), id); err != nil || d.Result != "allowed" {
		t.Fatalf("new resource delivery failed: %v", err)
	}
	revoked := "revoked"
	if _, err := s.UpdateCredential(ctx, c.ID, CredentialPatch{Status: &revoked}); err != nil {
		t.Fatal(err)
	}
	id = 1
	if _, err := s.UpdateCredential(ctx, c.ID, CredentialPatch{SubscriptionID: &id}); err != Inactive {
		t.Fatalf("revoked credential was editable: %v", err)
	}
}
