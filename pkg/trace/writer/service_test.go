package writer

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	writerconfig "github.com/DataDog/datadog-agent/pkg/trace/writer/config"
	"github.com/stretchr/testify/assert"
)

func TestServiceWriter_SenderMaxPayloads(t *testing.T) {
	assert := assert.New(t)

	// Given a service writer
	serviceWriter, _, _, _ := testServiceWriter()

	// When checking its default sender configuration
	queuableSender := serviceWriter.sender.(*queuableSender)

	// Then the MaxQueuedPayloads setting should be -1 (unlimited)
	assert.Equal(-1, queuableSender.conf.MaxQueuedPayloads)
}

func TestServiceWriter_ServiceHandling(t *testing.T) {
	assert := assert.New(t)

	// Given a service writer, its incoming channel and the endpoint that receives the payloads
	serviceWriter, serviceChannel, testEndpoint, _ := testServiceWriter()
	serviceWriter.conf.FlushPeriod = 100 * time.Millisecond

	serviceWriter.Start()

	// Given a set of service metadata
	metadata1 := testutil.RandomServices(10, 10)

	// When sending it
	serviceChannel <- metadata1

	// And then immediately sending another set of service metadata
	metadata2 := testutil.RandomServices(10, 10)
	serviceChannel <- metadata2

	// And then waiting for more than flush period
	time.Sleep(2 * serviceWriter.conf.FlushPeriod)

	// And then sending a third set of service metadata
	metadata3 := testutil.RandomServices(10, 10)
	serviceChannel <- metadata3

	// And stopping service writer before flush ticker ticks (should still flush on exit though)
	close(serviceChannel)
	serviceWriter.Stop()

	// Then the endpoint should have received 2 payloads, containing all sent metadata
	expectedHeaders := map[string]string{
		"X-Datadog-Reported-Languages": strings.Join(info.Languages(), "|"),
		"Content-Type":                 "application/json",
	}

	mergedMetadata := mergeMetadataInOrder(metadata1, metadata2)
	successPayloads := testEndpoint.SuccessPayloads()

	assert.Len(successPayloads, 2, "There should be 2 payloads")
	assertMetadata(assert, expectedHeaders, mergedMetadata, successPayloads[0])
	assertMetadata(assert, expectedHeaders, metadata3, successPayloads[1])
}

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

func mergeMetadataInOrder(metadatas ...pb.ServicesMetadata) pb.ServicesMetadata {
	result := pb.ServicesMetadata{}

	for _, metadata := range metadatas {
		for serviceName, serviceMetadata := range metadata {
			result[serviceName] = serviceMetadata
		}
	}

	return result
}

func calculateMetadataPayloadSize(metadata pb.ServicesMetadata) int64 {
	data, _ := json.Marshal(metadata)
	return int64(len(data))
}

func assertMetadata(assert *assert.Assertions, expectedHeaders map[string]string,
	expectedMetadata pb.ServicesMetadata, p *payload) {
	servicesMetadata := pb.ServicesMetadata{}

	assert.NoError(json.Unmarshal(p.bytes, &servicesMetadata), "Stats payload should unmarshal correctly")

	assert.Equal(expectedHeaders, p.headers, "Headers should match expectation")
	assert.Equal(expectedMetadata, servicesMetadata, "Service metadata should match expectation")
}

func testServiceWriter() (*ServiceWriter, chan pb.ServicesMetadata, *testEndpoint, *testutil.TestStatsClient) {
	serviceChannel := make(chan pb.ServicesMetadata)
	conf := &config.AgentConfig{
		ServiceWriterConfig: writerconfig.DefaultServiceWriterConfig(),
	}
	serviceWriter := NewServiceWriter(conf, serviceChannel)
	testEndpoint := &testEndpoint{}
	serviceWriter.sender.setEndpoint(testEndpoint)
	testStatsClient := metrics.Client.(*testutil.TestStatsClient)
	testStatsClient.Reset()

	return serviceWriter, serviceChannel, testEndpoint, testStatsClient
}
