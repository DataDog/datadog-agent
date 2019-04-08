// +build !windows

package writer

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/stretchr/testify/assert"
)

func TestServiceWriter_UpdateInfoHandling(t *testing.T) {
	rand.Seed(1)
	assert := assert.New(t)

	// Given a service writer, its incoming channel and the endpoint that receives the payloads
	serviceWriter, serviceChannel, testEndpoint, statsClient := testServiceWriter()
	serviceWriter.conf.FlushPeriod = 100 * time.Millisecond
	serviceWriter.conf.UpdateInfoPeriod = 100 * time.Millisecond

	serviceWriter.Start()

	expectedNumPayloads := int64(0)
	expectedNumServices := int64(0)
	expectedNumBytes := int64(0)
	expectedMinNumRetries := int64(0)
	expectedNumErrors := int64(0)

	// When sending a set of metadata
	expectedNumPayloads++
	metadata1 := testutil.RandomServices(10, 10)
	serviceChannel <- metadata1
	expectedNumServices += int64(len(metadata1))
	expectedNumBytes += calculateMetadataPayloadSize(metadata1)

	// And waiting for twice the flush period to trigger payload sending and info updating
	time.Sleep(2 * serviceWriter.conf.FlushPeriod)

	// And then sending a second set of metadata
	expectedNumPayloads++
	metadata2 := testutil.RandomServices(10, 10)
	serviceChannel <- metadata2
	expectedNumServices += int64(len(metadata2))
	expectedNumBytes += calculateMetadataPayloadSize(metadata2)

	// And waiting for twice the flush period to trigger payload sending and info updating
	time.Sleep(2 * serviceWriter.conf.FlushPeriod)

	// And then sending a third payload with other 3 traces with an errored out endpoint with no retry
	testEndpoint.SetError(fmt.Errorf("non retriable error"))
	expectedNumErrors++
	metadata3 := testutil.RandomServices(10, 10)
	serviceChannel <- metadata3
	expectedNumServices += int64(len(metadata3))
	expectedNumBytes += calculateMetadataPayloadSize(metadata3)

	// And waiting for twice the flush period to trigger payload sending and info updating
	time.Sleep(2 * serviceWriter.conf.FlushPeriod)

	// And then sending a third payload with other 3 traces with an errored out endpoint with retry
	testEndpoint.SetError(&retriableError{
		err:      fmt.Errorf("retriable error"),
		endpoint: testEndpoint,
	})
	expectedMinNumRetries++
	metadata4 := testutil.RandomServices(10, 10)
	serviceChannel <- metadata4
	expectedNumServices += int64(len(metadata4))
	expectedNumBytes += calculateMetadataPayloadSize(metadata4)

	// And waiting for twice the flush period to trigger payload sending and info updating
	time.Sleep(2 * serviceWriter.conf.FlushPeriod)

	close(serviceChannel)
	serviceWriter.Stop()

	// Then we expect some counts and gauges to have been sent to the stats client for each update tick (there should
	// have been at least 3 ticks)
	countSummaries := statsClient.GetCountSummaries()

	// Payload counts
	payloadSummary := countSummaries["datadog.trace_agent.service_writer.payloads"]
	assert.True(len(payloadSummary.Calls) >= 3, "There should have been multiple payload count calls")
	assert.Equal(expectedNumPayloads, payloadSummary.Sum)

	// Services count
	servicesSummary := countSummaries["datadog.trace_agent.service_writer.services"]
	assert.True(len(servicesSummary.Calls) >= 3, "There should have been multiple services gauge calls")
	assert.EqualValues(expectedNumServices, servicesSummary.Sum)

	// Bytes counts
	bytesSummary := countSummaries["datadog.trace_agent.service_writer.bytes"]
	assert.True(len(bytesSummary.Calls) >= 3, "There should have been multiple bytes count calls")
	assert.Equal(expectedNumBytes, bytesSummary.Sum)

	// Retry counts
	retriesSummary := countSummaries["datadog.trace_agent.service_writer.retries"]
	assert.True(len(retriesSummary.Calls) >= 2, "There should have been multiple retries count calls")
	assert.True(retriesSummary.Sum >= expectedMinNumRetries)

	// Error counts
	errorsSummary := countSummaries["datadog.trace_agent.service_writer.errors"]
	assert.True(len(errorsSummary.Calls) >= 3, "There should have been multiple errors count calls")
	assert.Equal(expectedNumErrors, errorsSummary.Sum)
}
