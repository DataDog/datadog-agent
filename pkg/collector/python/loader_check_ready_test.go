//go:build python

package python

import "testing"

func TestCheckReadyCheckNameTagValue(t *testing.T) {
	t.Run("uses loadedName when present", func(t *testing.T) {
		got := checkReadyCheckNameTagValue("redisdb", "datadog_checks.redisdb")
		if got != "datadog_checks.redisdb" {
			t.Fatalf("expected loadedName to win, got %q", got)
		}
	})

	t.Run("falls back to moduleName when loadedName empty", func(t *testing.T) {
		got := checkReadyCheckNameTagValue("redisdb", "")
		if got != "redisdb" {
			t.Fatalf("expected moduleName fallback, got %q", got)
		}
	})
}
