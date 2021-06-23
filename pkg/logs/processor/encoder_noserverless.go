// +build !serverless

package processor

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util"
)

// getHostname returns the name of the host.
func getHostname() string {
	hostname, err := util.GetHostname(context.TODO())
	if err != nil {
		// this scenario is not likely to happen since
		// the agent can not start without a hostname
		hostname = "unknown"
	}
	return hostname
}
