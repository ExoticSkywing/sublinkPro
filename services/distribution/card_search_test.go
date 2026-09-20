package distribution

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestCardSearchByCode(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	rows, err := s.CreateCards(ctx, CardInput{Name: "Support", Batch: "support-batch", Count: 2, Kind: "quarter"})
	if err != nil {
		t.Fatal(err)
	}
	want := rows[0]
	if err := s.UpdateCard(ctx, rows[1].ID, CardPatch{Name: &want.Code}); err != nil {
		t.Fatal(err)
	}
	before, err := s.CardByID(ctx, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	free, err := s.CurrentFreeCard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	legacyCode := strings.Repeat("ab12", 16)
	secret, err := s.seal(legacyCode)
	if err != nil {
		t.Fatal(err)
	}
	legacy := Card{Name: "Legacy", CodeHash: digest(legacyCode), Secret: secret, Enabled: true}
	if err := s.DB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		keyword string
		id      uint
	}{
		{want.Code, want.ID},
		{strings.ToLower(want.Code), want.ID},
		{strings.ReplaceAll(want.Code, "-", ""), want.ID},
		{" \n" + strings.ReplaceAll(want.Code, "-", " \t") + "\n", want.ID},
		{free.Code, free.ID},
		{strings.ToUpper(legacyCode), legacy.ID},
		{" \t" + legacyCode + "\n", legacy.ID},
	} {
		result, err := s.Cards(ctx, ListFilter{Keyword: tc.keyword})
		if err != nil || result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != tc.id {
			t.Fatal("code search failed to identify exact card", err)
		}
	}
	for _, keyword := range []string{strings.Repeat("0", 64), "' OR 1=1 --", free.Code[:8]} {
		result, err := s.Cards(ctx, ListFilter{Keyword: keyword})
		if err != nil || result.Total != 0 || len(result.Items) != 0 {
			t.Fatal("unknown or partial code matched", err)
		}
	}
	if _, err := s.Cards(ctx, ListFilter{Keyword: strings.Repeat("a", 2049)}); err != Invalid {
		t.Fatal("oversized search accepted", err)
	}
	if _, err := s.Cards(ctx, ListFilter{Status: "invalid"}); err != Invalid {
		t.Fatal("invalid status accepted", err)
	}
	after, err := s.CardByID(ctx, want.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("search changed card or redemption state", err)
	}
	var count int64
	if err := s.DB.Model(&Redemption{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("search created a redemption", err)
	}
	// An unrelated corrupt encrypted code must not break indexed search.
	if err := s.DB.Model(&Card{}).Where("id = ?", rows[1].ID).Update("secret", "invalid").Error; err != nil {
		t.Fatal(err)
	}
	if result, err := s.Cards(ctx, ListFilter{Keyword: want.Code}); err != nil || result.Total != 1 {
		t.Fatal("search scanned unrelated encrypted cards", err)
	}
}

func TestCardSearchFiltersDeletionAndPagination(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	rows, err := s.CreateCards(ctx, CardInput{Name: "Support", Batch: "support-batch", Count: 2, Kind: "year"})
	if err != nil {
		t.Fatal(err)
	}
	for _, keyword := range []string{" Support ", "support-batch"} {
		result, err := s.Cards(ctx, ListFilter{Keyword: keyword, Page: 2, Size: 1})
		if err != nil || result.Total != 2 || len(result.Items) != 1 || result.Items[0].ID != rows[0].ID {
			t.Fatal("name/batch search or pagination changed", err)
		}
	}
	if err := s.SetCardEnabled(ctx, rows[0].ID, false); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"", "enabled", "disabled"} {
		result, err := s.Cards(ctx, ListFilter{Keyword: rows[0].Code, Status: status})
		want := int64(1)
		if status == "enabled" {
			want = 0
		}
		if err != nil || result.Total != want {
			t.Fatal("code search bypassed status filter", err)
		}
	}
	if err := s.DeleteCards(ctx, []uint{rows[0].ID}); err != nil {
		t.Fatal(err)
	}
	result, err := s.Cards(ctx, ListFilter{Keyword: rows[0].Code})
	if err != nil || result.Total != 0 {
		t.Fatal("deleted card exposed by search", err)
	}
}
