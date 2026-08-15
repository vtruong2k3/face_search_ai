package config

import "testing"

func TestLoadDatabaseSettings(t *testing.T) {
	t.Setenv("DATABASE_MAX_CONNECTIONS", "24")
	t.Setenv("DATABASE_SCHEMA_VERSION", "7")
	cfg := Load()
	if cfg.DatabaseMaxConns != 24 {
		t.Fatalf("DatabaseMaxConns = %d", cfg.DatabaseMaxConns)
	}
	if cfg.SchemaVersion != 7 {
		t.Fatalf("SchemaVersion = %d", cfg.SchemaVersion)
	}
}

func TestLoadDatabaseSettingsUsesSafeDefaults(t *testing.T) {
	t.Setenv("DATABASE_MAX_CONNECTIONS", "0")
	t.Setenv("DATABASE_SCHEMA_VERSION", "invalid")
	cfg := Load()
	if cfg.DatabaseMaxConns != 10 {
		t.Fatalf("DatabaseMaxConns = %d", cfg.DatabaseMaxConns)
	}
	if cfg.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d", cfg.SchemaVersion)
	}
}
