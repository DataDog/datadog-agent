package writer

import (
	"encoding/json"
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
