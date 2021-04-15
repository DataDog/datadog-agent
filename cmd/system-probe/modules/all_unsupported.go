// +build !linux,!windows

package modules

import "github.com/DataDog/datadog-agent/cmd/system-probe/api"

// All System Probe modules should register their factories here
var All = []api.Factory{}
