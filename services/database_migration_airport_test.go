package services

import (
	"testing"

	"sublink/internal/testutil"
	"sublink/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestImportAirportsSupportsBackupWithoutFilterSummaryColumn(t *testing.T) {
	source, err := gorm.Open(sqlite.Open(testutil.UniqueMemoryDSN(t, "airport_import_source")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open source db: %v", err)
	}
	target, err := gorm.Open(sqlite.Open(testutil.UniqueMemoryDSN(t, "airport_import_target")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open target db: %v", err)
	}
	t.Cleanup(func() {
		testutil.CloseDB(t, source)
		testutil.CloseDB(t, target)
	})

	if err := source.Exec(`CREATE TABLE airports (
		id INTEGER PRIMARY KEY,
		name TEXT,
		url TEXT,
		cron_expr TEXT,
		enabled NUMERIC,
		success_count INTEGER
	)`).Error; err != nil {
		t.Fatalf("create legacy airports table: %v", err)
	}
	if err := source.Exec(
		"INSERT INTO airports (id, name, url, cron_expr, enabled, success_count) VALUES (?, ?, ?, ?, ?, ?)",
		7, "旧版机场", "https://example.com/legacy", "0 */12 * * *", true, 6,
	).Error; err != nil {
		t.Fatalf("insert legacy airport: %v", err)
	}
	if err := target.AutoMigrate(&models.Airport{}); err != nil {
		t.Fatalf("migrate target airports: %v", err)
	}

	result := &DatabaseMigrationResult{Imported: make(map[string]int)}
	state := &databaseMigrationState{source: source, tx: target, result: result}
	if err := importAirports(state); err != nil {
		t.Fatalf("import legacy airports: %v", err)
	}

	var airport models.Airport
	if err := target.First(&airport, 7).Error; err != nil {
		t.Fatalf("reload imported airport: %v", err)
	}
	if airport.Name != "旧版机场" || airport.SuccessCount != 6 {
		t.Fatalf("unexpected imported airport: %+v", airport)
	}
	if airport.NodeFilterSummary != "" {
		t.Fatalf("legacy airport filter summary = %q, want empty", airport.NodeFilterSummary)
	}
	if result.Imported["airports"] != 1 {
		t.Fatalf("imported airports = %d, want 1", result.Imported["airports"])
	}
}
