package updater

import (
	"github.com/DataDog/datadog-agent/comp/updater/updater"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: fleet

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
	return fxutil.Bundle(
		updater.Module(),
	)
}
