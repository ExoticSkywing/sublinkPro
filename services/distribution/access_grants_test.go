package distribution

import (
	"context"
	"testing"
	"time"
)

func TestAccessGrantAllowsOnlyExactLocationException(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	c := issueOne(t, s)
	activate(t, s, c)
	cfg, _ := s.Settings(ctx)
	cfg.RegionLimit = 1
	if err := s.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	expires := s.Now().Add(time.Hour)
	grant, err := s.CreateAccessGrant(ctx, c.ID, AccessGrantInput{IP: "8.8.8.8", ExpiresAt: expires, Reason: "Resin"})
	if err != nil {
		t.Fatal(err)
	}
	if grant.IP != "8.8.8.8" || !grant.Enabled {
		t.Fatalf("unexpected grant: %+v", grant)
	}
	if d, err := s.Check(ctx, c.Token, testCity(2), "8.8.8.9"); err != nil || d.Result != "region_denied" || d.Emergency {
		t.Fatalf("non-matching IP bypassed region policy: %+v %v", d, err)
	}
	d, err := s.Check(ctx, c.Token, testCity(2), "8.8.8.8")
	if err != nil || d.Result != "allowed" || !d.Emergency {
		t.Fatalf("matching grant did not allow: %+v %v", d, err)
	}
	d, err = s.CommitDelivery(ctx, c.Token, testCity(2), c.SubscriptionID, "8.8.8.8")
	if err != nil || d.Result != "allowed" || !d.Emergency {
		t.Fatalf("emergency delivery failed: %+v %v", d, err)
	}
	var regions []Region
	if err := s.DB.Where("credential_id = ?", c.ID).Find(&regions).Error; err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 {
		t.Fatalf("emergency delivery consumed a region slot: %+v", regions)
	}
	var stored AccessGrant
	if err := s.DB.First(&stored, grant.ID).Error; err != nil || stored.LastUsedAt == nil {
		t.Fatalf("grant use was not recorded: %+v %v", stored, err)
	}
}

func TestAccessGrantDoesNotBypassExpiryOrRevocation(t *testing.T) {
	s, now := testStore(t)
	ctx := context.Background()
	c := issueOne(t, s)
	activate(t, s, c)
	grant, err := s.CreateAccessGrant(ctx, c.ID, AccessGrantInput{IP: "8.8.8.8", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	past := now.Add(-time.Minute)
	if _, err := s.UpdateCredential(ctx, c.ID, CredentialPatch{ExpiresAt: &past}); err != nil {
		t.Fatal(err)
	}
	if d, err := s.Check(ctx, c.Token, City{}, "8.8.8.8"); err != nil || d.Result != "location_unavailable" || d.Emergency {
		t.Fatalf("expired credential used emergency grant: %+v %v", d, err)
	}
	if err := s.RevokeAccessGrant(ctx, c.ID, grant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAccessGrant(ctx, c.ID, AccessGrantInput{IP: "10.0.0.1", ExpiresAt: now.Add(time.Hour)}); err != Invalid {
		t.Fatalf("private IP accepted: %v", err)
	}
	for _, ip := range []string{"100.64.0.1", "192.0.2.1", "198.18.0.1", "2001:db8::1"} {
		if _, err := s.CreateAccessGrant(ctx, c.ID, AccessGrantInput{IP: ip, ExpiresAt: now.Add(time.Hour)}); err != Invalid {
			t.Fatalf("special-use IP %s accepted: %v", ip, err)
		}
	}
}

func TestAccessGrantEmergencyWithoutCityDoesNotReserveRegion(t *testing.T) {
	s, now := testStore(t)
	ctx := context.Background()
	c := issueOne(t, s)
	grant, err := s.CreateAccessGrant(ctx, c.ID, AccessGrantInput{IP: "8.8.8.8", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.CommitDelivery(ctx, c.Token, City{}, c.SubscriptionID, grant.IP)
	if err != nil || d.Result != "allowed" || !d.Emergency {
		t.Fatalf("emergency delivery failed: %+v %v", d, err)
	}
	var regions []Region
	if err := s.DB.Where("credential_id = ?", c.ID).Find(&regions).Error; err != nil {
		t.Fatal(err)
	}
	if len(regions) != 0 {
		t.Fatalf("unresolved city consumed region slot: %+v", regions)
	}
}
