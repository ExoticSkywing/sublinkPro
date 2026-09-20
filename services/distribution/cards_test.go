package distribution

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNewCardCodesAndNormalizedRedemption(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	owner, other := issueOne(t, s), issueOne(t, s)
	activate(t, s, owner)
	activate(t, s, other)
	if len(owner.Token) != 64 {
		t.Fatal("card format changed subscription tokens")
	}
	cards, err := s.CreateCards(ctx, CardInput{Name: "Codes", Batch: "codes", Count: 100, Kind: "quarter"})
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`^[2-9A-HJ-NP-Z]{4}(-[2-9A-HJ-NP-Z]{4}){3}$`)
	seen := map[string]bool{}
	for _, card := range cards {
		if !pattern.MatchString(card.Code) || seen[card.Code] {
			t.Fatal("invalid or duplicate card format")
		}
		seen[card.Code] = true
		if card.CodeHash != digest(normalizeCardCode(card.Code)) || card.Secret == card.Code {
			t.Fatal("card storage is not normalized and encrypted")
		}
	}
	code := cards[0].Code
	var original Redemption
	for _, input := range []string{code, strings.ToLower(code), strings.ReplaceAll(code, "-", ""), " \t" + strings.ReplaceAll(code, "-", " \n") + "\r\n"} {
		r, err := s.Redeem(ctx, owner.Token, input, "web")
		if err != nil {
			t.Fatal(err)
		}
		if original.ID == 0 {
			original = r
		} else if r.ID != original.ID || !r.After.Equal(*original.After) {
			t.Fatal("alternate spelling extended the same card twice")
		}
	}
	if _, err := s.Redeem(ctx, other.Token, strings.ToLower(code), "api"); err != CardUsed {
		t.Fatalf("paid card lost its owner: %v", err)
	}
	listed, err := s.Cards(ctx, ListFilter{Size: 100})
	if err != nil || len(listed.Items) != 100 {
		t.Fatalf("list cards: %v", err)
	}
	for _, card := range listed.Items {
		if !seen[card.Code] {
			t.Fatal("list changed the exported code")
		}
	}
}

func TestLegacyCardCodesAndFreeCycleArePreserved(t *testing.T) {
	for _, kind := range []string{"quarter", "free"} {
		t.Run(kind, func(t *testing.T) {
			s, now := testStore(t)
			ctx := context.Background()
			owner := issueOne(t, s)
			activate(t, s, owner)
			*now = now.Add(16 * 24 * time.Hour)
			var card Card
			if kind == "free" {
				var err error
				card, err = s.CurrentFreeCard(ctx)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				cards, err := s.CreateCards(ctx, CardInput{Name: "Legacy", Batch: "legacy", Count: 1, Kind: kind})
				if err != nil {
					t.Fatal(err)
				}
				card = cards[0]
			}
			// Simulate pre-upgrade storage without rewriting the actual database.
			legacy := strings.Repeat("ab12", 16)
			secret, err := s.seal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.DB.Model(&Card{}).Where("id = ?", card.ID).Updates(map[string]any{"secret": secret, "code_hash": digest(legacy)}).Error; err != nil {
				t.Fatal(err)
			}
			first, err := s.Redeem(ctx, owner.Token, " \n"+strings.ToUpper(legacy)+"\t", "web")
			if err != nil {
				t.Fatal(err)
			}
			again, err := s.Redeem(ctx, owner.Token, legacy, "api")
			if err != nil || again.ID != first.ID {
				t.Fatal("legacy redemption is not idempotent", err)
			}
			if kind == "free" {
				same, err := s.CurrentFreeCard(ctx)
				if err != nil || same.ID != card.ID || same.Code != legacy || same.RedemptionCount != 1 {
					t.Fatal("current legacy free code was replaced", err)
				}
				*now = now.Add(7 * 24 * time.Hour)
				next, err := s.CurrentFreeCard(ctx)
				if err != nil || next.ID == card.ID || len(next.Code) != 19 {
					t.Fatal("next cycle did not use the new format", err)
				}
			} else {
				same, err := s.CreateCards(ctx, CardInput{Name: "Legacy", Batch: "legacy", Count: 1, Kind: kind})
				if err != nil || same[0].Code != legacy {
					t.Fatal("batch retry replaced legacy code", err)
				}
			}
		})
	}
}
