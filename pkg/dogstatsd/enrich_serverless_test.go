// +build serverless

package dogstatsd

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestConvertParseDistributionServerless(t *testing.T) {
	defaultHostname, err := util.GetHostname(context.Background())

	assert.Equal(t, "", defaultHostname, "In serverless mode, the hostname returned should be an empty string")
	assert.NoError(t, err)

	parsed, err := parseAndEnrichMetricMessage([]byte("daemon:3.5|d"), "", nil, defaultHostname)

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 3.5, parsed.Value)
	assert.Equal(t, metrics.DistributionType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))

	// this is the important part of the test: util.GetHostname() should return
	// an empty string and the parser / enricher should keep the host that way.
	assert.Equal(t, "", parsed.Host, "In serverless mode, the hostname should be an empty string")
}
