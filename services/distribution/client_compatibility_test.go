package distribution

import (
	"context"
	"reflect"
	"regexp"
	"testing"
)

func TestClashMetaAndroidDefaultAllowlist(t *testing.T) {
	s, _ := testStore(t)
	cfg, err := s.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	matcher, err := regexp.Compile(cfg.AllowedUA)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ua   string
		want bool
	}{
		{"ClashMetaForAndroid/2.11.25.Meta", true},
		{"clashmetaforandroid/2.11.25.meta", true},
		{"ClashMetaForAndroid", true},
		{"Clash/1.0", true},
		{"clash-verge/v2.5.2", true},
		{"mihomo/1.19", true},
		{"Shadowrocket/2.2", true},
		{"ClashMetaForAndroidOther/1", false},
		{"OtherClashMetaForAndroid/1", false},
		{"Mozilla/5.0 ClashMetaForAndroid/2.11.25.Meta", false},
		{"curl/8.0", false},
		{"Dart/3.0", false},
		{"", false},
	} {
		if got := matcher.MatchString(tc.ua); got != tc.want {
			t.Errorf("UA %q: got %v, want %v", tc.ua, got, tc.want)
		}
	}
}

func TestClashMetaAndroidMigrationPreservesCustomSettings(t *testing.T) {
	for _, tc := range []struct{ name, original string }{
		{"legacy-default", legacyDefaultAllowedUA},
		{"current-default", defaultAllowedUA},
		{"custom", `(?i)^clash/`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := testStore(t)
			ctx := context.Background()
			cfg, err := s.Settings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			cfg.AllowedUA = tc.original
			cfg.TrialDays = 23
			cfg.Domain = "custom.example"
			cfg.ExpiredMessages = []string{"Custom notice"}
			if err := s.SaveSettings(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			want, err := s.Settings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if tc.original == legacyDefaultAllowedUA {
				want.AllowedUA = defaultAllowedUA
			}
			for i := 0; i < 2; i++ {
				if err := s.Migrate(); err != nil {
					t.Fatal(err)
				}
				got, err := s.Settings(ctx)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatal("migration changed unrelated settings or failed to preserve the allowlist", err)
				}
			}
		})
	}
}
