package security

import "fmt"

// SupportedCanonicalSchema is the canonical model version this binary reads and writes.
const SupportedCanonicalSchema = 1

// CheckDatabaseCompatibility fails closed on unknown schema versions.
func CheckDatabaseCompatibility(canonical int, migrationVersion int) error {
	if migrationVersion < 0 {
		return fmt.Errorf("invalid migration version %d", migrationVersion)
	}
	if canonical == 0 && migrationVersion == 0 {
		return nil
	}
	if canonical > SupportedCanonicalSchema {
		return fmt.Errorf("database canonical_schema_version %d is newer than this binary (supports up to %d); upgrade the CLI", canonical, SupportedCanonicalSchema)
	}
	if canonical > 0 && canonical < SupportedCanonicalSchema {
		return fmt.Errorf("database canonical_schema_version %d requires migration; see docs/UPGRADE.md", canonical)
	}
	return nil
}
