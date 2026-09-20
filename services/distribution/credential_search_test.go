package distribution

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestCredentialSearchByLinkAndToken(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	want, decoy := issueOne(t, s), issueOne(t, s)
	// A token-shaped name must never turn an exact lookup into multiple users.
	if _, err := s.UpdateCredential(ctx, decoy.ID, CredentialPatch{Name: &want.Token}); err != nil {
		t.Fatal(err)
	}
	before, err := s.ByID(ctx, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	link := "https://subscription.example/d/" + want.Token
	for _, keyword := range []string{want.Token, strings.ToUpper(want.Token), link, " \t" + link + "?client=mihomo#client-name\n", link + "/?client=v2ray", "http://old-domain.example/d/" + want.Token} {
		result, err := s.Credentials(ctx, ListFilter{Keyword: keyword})
		if err != nil || result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != want.ID {
			t.Fatal("link/token search did not identify the exact recipient", err)
		}
	}
	for _, keyword := range []string{strings.Repeat("0", 64), "https://example.invalid/c/?token=" + want.Token, link + "/extra", "https://user:password@example.invalid/d/" + want.Token, "https://example.invalid/d/incomplete", "' OR 1=1 --"} {
		result, err := s.Credentials(ctx, ListFilter{Keyword: keyword})
		if err != nil || result.Total != 0 || len(result.Items) != 0 {
			t.Fatal("unknown, partial or invalid link matched a recipient", err)
		}
	}
	// Partial input remains a name search, not a scan of decrypted tokens.
	partial, err := s.Credentials(ctx, ListFilter{Keyword: want.Token[:32]})
	if err != nil || partial.Total != 1 || partial.Items[0].ID != decoy.ID {
		t.Fatal("partial input searched token contents instead of names", err)
	}
	longLink := link + "?padding=" + strings.Repeat("x", 2048-len(link)-len("?padding="))
	if result, err := s.Credentials(ctx, ListFilter{Keyword: longLink}); err != nil || result.Total != 1 {
		t.Fatal("maximum length link not searchable", err)
	}
	if _, err := s.Credentials(ctx, ListFilter{Keyword: longLink + "x"}); err != Invalid {
		t.Fatal("oversized search must be rejected, not truncated", err)
	}
	after, err := s.ByID(ctx, want.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("search changed activation, access count, rights or regions", err)
	}
}

func TestCredentialSearchPreservesFiltersAndRotation(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	rows, err := s.Issue(ctx, IssueInput{Name: "Customer", Batch: "support-batch", Count: 2, SubscriptionID: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, keyword := range []string{"Customer", "support-batch", " Customer "} {
		result, err := s.Credentials(ctx, ListFilter{Keyword: keyword, Page: 2, Size: 1})
		if err != nil || result.Total != 2 || len(result.Items) != 1 || result.Items[0].ID != rows[0].ID {
			t.Fatal("name/batch pagination changed", err)
		}
	}
	row := rows[0]
	if _, err := s.UpdateCredential(ctx, row.ID, CredentialPatch{Status: ptr("disabled")}); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"", "disabled", "enabled", "revoked"} {
		result, err := s.Credentials(ctx, ListFilter{Keyword: row.Token, Status: status})
		want := int64(0)
		if status == "" || status == "disabled" {
			want = 1
		}
		if err != nil || result.Total != want {
			t.Fatal("exact search bypassed status filter", err)
		}
	}
	rotated, err := s.UpdateCredential(ctx, row.ID, CredentialPatch{Rotate: true})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := s.Credentials(ctx, ListFilter{Keyword: row.Token}); err != nil || result.Total != 0 {
		t.Fatal("rotated-out token remained searchable", err)
	}
	if result, err := s.Credentials(ctx, ListFilter{Keyword: rotated.Token}); err != nil || result.Total != 1 || result.Items[0].ID != row.ID {
		t.Fatal("replacement link did not find original recipient", err)
	}
	if err := s.DeleteCredentials(ctx, []uint{row.ID}); err != nil {
		t.Fatal(err)
	}
	if result, err := s.Credentials(ctx, ListFilter{Keyword: rotated.Token}); err != nil || result.Total != 0 {
		t.Fatal("default search exposed deleted record", err)
	}
	deleted, err := s.Credentials(ctx, ListFilter{Keyword: rotated.Token, Status: "revoked"})
	if err != nil || deleted.Total != 1 || deleted.Items[0].ID != row.ID || deleted.Items[0].Token != "" {
		t.Fatal("deleted search did not preserve token-free audit lookup", err)
	}
}
