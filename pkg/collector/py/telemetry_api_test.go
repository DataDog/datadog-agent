package py

import (
	"github.com/StackVista/stackstate-agent/pkg/aggregator/mocksender"
	"github.com/StackVista/stackstate-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
)

func TestSubmitTopologyEvent(t *testing.T) {
	check, _ := getCheckInstance("testtelemetry", "TestTopologyEvents")

	mockSender := mocksender.NewMockSender(check.ID())

	mockSender.On("Event", mock.AnythingOfType("metrics.Event")).Return().Once()
	mockSender.On("Commit").Return().Times(1)

	err := check.Run()
	assert.Nil(t, err)

	sentTopologyEvent := *mockSender.SentEvents[0]
	assert.Equal(t, "URL timeout", sentTopologyEvent.Title)
	assert.Equal(t, "Http request to agent-integration-sample timed out after 5.0 seconds.", sentTopologyEvent.Text)
	assert.Equal(t, "instance-request-agent-integration-sample", sentTopologyEvent.AggregationKey)
	assert.Equal(t, "HTTP_TIMEOUT", sentTopologyEvent.SourceTypeName)
	expectedEventContext := &metrics.EventContext{
		SourceIdentifier:   "source_identifier_value",
		ElementIdentifiers: []string{"urn:host:/123"},
		Source:             "source_value",
		Category:           "my_category",
		Data:               map[string]interface{}{"big_black_hole": "here"},
		SourceLinks:        []metrics.SourceLink{{Title: "my_event_external_link", URL: "http://localhost"}},
	}
	assert.EqualValues(t, expectedEventContext, sentTopologyEvent.EventContext)
}
