package store

import (
	"strings"
	"testing"
)

func TestCheckDuplicateVersions(t *testing.T) {
	t.Run("distinct versions pass", func(t *testing.T) {
		names := []string{
			"0001_init.sql",
			"0002_glicko2.sql",
			"0010_country_array.sql",
		}
		if err := checkDuplicateVersions(names); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("collision is reported, not skipped", func(t *testing.T) {
		// The exact incident that left production un-migrated: two files at 0009.
		names := []string{
			"0009_country_array.sql",
			"0009_proposals.sql",
		}
		err := checkDuplicateVersions(names)
		if err == nil {
			t.Fatal("expected an error for the duplicate version 9, got nil")
		}
		wantParts := []string{
			"9", "0009_country_array.sql", "0009_proposals.sql",
		}
		for _, want := range wantParts {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q should name %q", err, want)
			}
		}
	})

	t.Run("non-numeric version surfaces the parse error", func(t *testing.T) {
		if err := checkDuplicateVersions([]string{"bad-name.sql"}); err == nil {
			t.Fatal("expected an error for a non-numeric version, got nil")
		}
	})
}
