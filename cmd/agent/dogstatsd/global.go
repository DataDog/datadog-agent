package dogstatsd

import (
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server"
)

// DSD is the global dogstatsd instance
var DSD server.Component
