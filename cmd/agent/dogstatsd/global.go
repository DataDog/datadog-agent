package dogstatsd

import (
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server"
)

// DSD is the global dogstatsd instance
// TODO: (components) This should be removed when all downstream dependencies
// are migrated to components such that this can be injected instead of a shared
// global reference.
var DSD server.Component
