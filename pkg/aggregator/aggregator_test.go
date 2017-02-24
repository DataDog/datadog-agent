package aggregator

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
)

var checkID1 check.ID = "1"
var checkID2 check.ID = "2"

func TestRegisterCheckSampler(t *testing.T) {
	resetAggregator()

	agg := InitAggregator(nil)
	err := agg.registerSender(checkID1)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	err = agg.registerSender(checkID2)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 2)

	// Already registered sender => error
	err = agg.registerSender(checkID2)
	assert.NotNil(t, err)
}

func TestDeregisterCheckSampler(t *testing.T) {
	resetAggregator()

	agg := InitAggregator(nil)
	agg.registerSender(checkID1)
	agg.registerSender(checkID2)
	assert.Len(t, aggregatorInstance.checkSamplers, 2)

	agg.deregisterSender(checkID1)
	if assert.Len(t, aggregatorInstance.checkSamplers, 1) {
		_, ok := agg.checkSamplers[checkID1]
		assert.False(t, ok)
		_, ok = agg.checkSamplers[checkID2]
		assert.True(t, ok)
	}
}

func TestAddServiceCheckDefaultValues(t *testing.T) {
	resetAggregator()
	agg := InitAggregator(nil)

	// For now the default hostname is the one pulled from the main config
	config.Datadog.Set("hostname", "config-hostname")

	agg.addServiceCheck(ServiceCheck{
		// leave Host and Ts fields blank
		CheckName: "my_service.can_connect",
		Status:    ServiceCheckOK,
		Tags:      []string{"foo", "bar"},
		Message:   "message",
	})
	agg.addServiceCheck(ServiceCheck{
		CheckName: "my_service.can_connect",
		Status:    ServiceCheckOK,
		Host:      "my-hostname",
		Tags:      []string{"foo", "bar"},
		Ts:        12345,
		Message:   "message",
	})

	if assert.Len(t, agg.serviceChecks, 2) {
		assert.Equal(t, "config-hostname", agg.serviceChecks[0].Host)
		assert.NotEqual(t, 0., agg.serviceChecks[0].Ts) // should be set to the current time, let's just check that it's not 0
		assert.Equal(t, "my-hostname", agg.serviceChecks[1].Host)
		assert.Equal(t, int64(12345), agg.serviceChecks[1].Ts)
	}
}
