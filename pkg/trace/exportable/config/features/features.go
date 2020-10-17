package features

import (
	"os"
	"strings"
)

// HasFeature returns true if the feature f is present. Features are values
// of the DD_APM_FEATURES environment variable.
func HasFeature(f string) bool {
	return strings.Contains(os.Getenv("DD_APM_FEATURES"), f)
}
