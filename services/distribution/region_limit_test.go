package distribution

import (
	"context"
	"sync"
	"testing"
)

func TestRegionLimitMigrationAndSettingsCompatibility(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	cfg, _ := s.Settings(ctx)
	if cfg.RegionLimit != 2 {
		t.Fatal("new installs must default to two cities")
	}
	if err := s.DB.Migrator().DropColumn(&Settings{}, "region_limit"); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	migrated, err := s.Settings(ctx)
	if err != nil || migrated.RegionLimit != 2 || migrated.TrialDays != cfg.TrialDays || migrated.AllowedUA != cfg.AllowedUA {
		t.Fatal("migration did not preserve legacy settings", err)
	}
	cfg.RegionLimit = 1
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	cfg.RegionLimit = 0 // Older API consumers omit the new field.
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	for _, value := range []int{-1, 3} {
		cfg.RegionLimit = value
		if err := s.SaveSettings(ctx, cfg); err != Invalid {
			t.Fatal("invalid city limit accepted", err)
		}
	}
	stored, err := s.Settings(ctx)
	if err != nil || stored.RegionLimit != 1 {
		t.Fatal("migration, omitted field or invalid write reset limit", err)
	}
}

func TestRegionLimitRecheckedForDeliveryAndApproval(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := issueOne(t, s)
	activate(t, s, c)
	if d, err := s.Check(ctx, c.Token, testCity(2)); err != nil || d.Result != "allowed" {
		t.Fatal("default limit changed", err)
	}
	cfg, _ := s.Settings(ctx)
	cfg.RegionLimit = 1
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	for _, check := range []func() (Decision, error){
		func() (Decision, error) { return s.Check(ctx, c.Token, testCity(2)) },
		func() (Decision, error) { return s.CommitDelivery(ctx, c.Token, testCity(2), c.SubscriptionID) },
	} {
		if d, err := check(); err != nil || d.Result != "region_denied" {
			t.Fatal("changed limit did not reject second city", err)
		}
	}
	if d, err := s.CommitDelivery(ctx, c.Token, testCity(1), c.SubscriptionID); err != nil || d.Result != "allowed" {
		t.Fatal("existing city stopped working", err)
	}
	state, err := s.PublicStatus(ctx, c.Token)
	if err != nil || state.RegionLimit != 1 || len(state.Regions) != 1 {
		t.Fatal("public count or bindings inconsistent", err)
	}
	city := testCity(2)
	visit := Visit{CredentialID: c.ID, Country: city.Country, CityKey: city.Key, City: city.Name, Result: "region_denied", CreatedAt: s.Now()}
	if err := s.DB.Create(&visit).Error; err != nil {
		t.Fatal(err)
	}
	request, err := s.ApplyRegion(ctx, ApplyInput{Link: c.Token, VisitID: visit.ID, Reason: "Travel to another city"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReviewRegion(ctx, request.ID, ReviewInput{Approve: true}, "test"); err != Invalid {
		t.Fatal("approval added a city above limit", err)
	}
	if err := s.ReviewRegion(ctx, request.ID, ReviewInput{Approve: true, ReplaceRegionID: state.Regions[0].ID}, "test"); err != nil {
		t.Fatal(err)
	}
	if d, err := s.Check(ctx, c.Token, testCity(1)); err != nil || d.Result != "region_denied" {
		t.Fatal("replaced city still allowed", err)
	}
	if d, err := s.CommitDelivery(ctx, c.Token, testCity(2), c.SubscriptionID); err != nil || d.Result != "allowed" {
		t.Fatal("approved city denied", err)
	}
	cfg.RegionLimit = 2
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if d, err := s.CommitDelivery(ctx, c.Token, testCity(3), c.SubscriptionID); err != nil || d.Result != "allowed" {
		t.Fatal("restoring limit failed", err)
	}
	cfg.RegionLimit = 1
	cfg.TrialDays = 99
	if err := s.SaveSettings(ctx, cfg); err != RegionLimitConflict {
		t.Fatal("lowering limit silently invalidated existing bindings", err)
	}
	state, err = s.PublicStatus(ctx, c.Token)
	if err != nil || state.RegionLimit != 2 || len(state.Regions) != 2 {
		t.Fatal("rejected setting deleted bindings or changed limit", err)
	}
	stored, err := s.Settings(ctx)
	if err != nil || stored.TrialDays == 99 {
		t.Fatal("rejected settings write changed other fields", err)
	}
	if err := s.DeleteCredentials(ctx, []uint{c.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal("deleted subscription blocked lowering city limit", err)
	}
}

func TestOneCityConcurrentFirstDelivery(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	cfg, _ := s.Settings(ctx)
	cfg.RegionLimit = 1
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	c := issueOne(t, s)
	var wg sync.WaitGroup
	for i := 1; i <= 4; i++ {
		wg.Add(1)
		go func(city int) {
			defer wg.Done()
			d, err := s.CommitDelivery(ctx, c.Token, testCity(city), c.SubscriptionID)
			if err != nil || (d.Result != "allowed" && d.Result != "region_denied") {
				t.Error("unexpected concurrent delivery result", err)
			}
		}(i)
	}
	wg.Wait()
	state, err := s.PublicStatus(ctx, c.Token)
	if err != nil || len(state.Regions) != 1 || state.ActivatedAt == nil {
		t.Fatal("concurrent first fetch bypassed one-city cap", err)
	}
}
