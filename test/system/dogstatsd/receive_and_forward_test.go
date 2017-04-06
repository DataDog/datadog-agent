package dogstatsd_test

import (
	"encoding/json"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

func TestReceiveAndForward(t *testing.T) {
	d := setupDogstatsd(t)
	defer d.teardown()
	defer log.Flush()

	d.sendUDP("_sc|test.ServiceCheck|0")

	timeOut := time.Tick(30 * time.Second)
	select {
	case <-d.requestReady:
	case <-timeOut:
		require.Fail(t, "Timeout: the backend never receive a requests from dogstatsd")
	}

	requests := d.getRequests()
	require.Len(t, requests, 1)

	sc := []aggregator.ServiceCheck{}
	err := json.Unmarshal([]byte(requests[0]), &sc)
	require.NoError(t, err, "Could not Unmarshal request")

	require.Len(t, sc, 2)
	assert.Equal(t, sc[0].CheckName, "test.ServiceCheck")
	assert.Equal(t, sc[0].Status, aggregator.ServiceCheckOK)

	assert.Equal(t, sc[1].CheckName, "datadog.agent.up")
	assert.Equal(t, sc[1].Status, aggregator.ServiceCheckOK)
}
