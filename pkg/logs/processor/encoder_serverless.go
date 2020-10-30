// +build serverless

package processor

import (
	"github.com/DataDog/datadog-agent/pkg/serverless/arn"
)

// getHostname returns the ARN of the executed function.
func getHostname() string {
	return arn.Get()
}
