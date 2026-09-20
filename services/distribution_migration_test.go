package services

import (
	"sublink/internal/testutil"
	"sublink/services/distribution"
	"testing"
)

func TestLegacyMigrationRefusesDistributionDataBeforeClearing(t *testing.T) {
	target := testutil.OpenMemoryDB(t, "distribution_migration_target")
	source := testutil.OpenMemoryDB(t, "distribution_migration_source")
	t.Cleanup(func() { testutil.CloseDB(t, target); testutil.CloseDB(t, source) })
	if err := ensureDistributionMigrationSafe(target, source); err != nil {
		t.Fatal(err)
	}
	if err := source.AutoMigrate(&distribution.Card{}); err != nil {
		t.Fatal(err)
	}
	if err := source.Create(&distribution.Card{Name: "preserve me", CodeHash: "unique"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureDistributionMigrationSafe(target, source); err == nil {
		t.Fatal("source distribution records ignored")
	}
	if err := ensureDistributionMigrationSafe(source, target); err == nil {
		t.Fatal("target distribution records ignored")
	}
	var count int64
	if err := source.Model(&distribution.Card{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("guard changed data")
	}
	if err := source.Where("code_hash = ?", "unique").Delete(&distribution.Card{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureDistributionMigrationSafe(target, source); err == nil {
		t.Fatal("deleted card audit ignored by migration guard")
	}
}
