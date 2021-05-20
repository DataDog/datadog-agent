package testutil

import (
	"os"

	featuresConfig "github.com/DataDog/datadog-agent/pkg/trace/export/config/features"
)

// WithFeatures sets the given list of comma-separated features as active and
// returns a function which resets the features to their previous state.
func WithFeatures(feats string) (undo func()) {
	featuresConfig.SetFeatures(feats)
	return func() {
		featuresConfig.SetFeatures(os.Getenv("DD_APM_FEATURES"))
	}
}
