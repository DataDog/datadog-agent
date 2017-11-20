package mocksender

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
)

func TestExpectedInActual(t *testing.T) {
	assert.True(t, expectedInActual([]string{}, []string{}))
	assert.True(t, expectedInActual([]string{}, []string{"one"}))
	assert.True(t, expectedInActual([]string{"one"}, []string{"one"}))
	assert.True(t, expectedInActual([]string{}, []string{"one", "two"}))
	assert.True(t, expectedInActual([]string{"one"}, []string{"one", "two"}))
	assert.True(t, expectedInActual([]string{"one", "two"}, []string{"one", "two"}))
	assert.True(t, expectedInActual([]string{"one", "two"}[0:0], []string{"one", "two"}))
	assert.True(t, expectedInActual([]string{"one", "two"}[:1], []string{"one", "two"}))
	assert.True(t, expectedInActual([]string{"one", "two"}[:1], []string{"one", "two"}[:1]))

	assert.False(t, expectedInActual([]string{"one", "two", "three"}, []string{"one", "two"}))
	assert.False(t, expectedInActual([]string{"one", "two", "three"}, []string{"one", "two", "four"}))
	assert.False(t, expectedInActual([]string{"one", "two", "three"}, []string{}))
	assert.False(t, expectedInActual([]string{"one"}, []string{}))
	assert.False(t, expectedInActual([]string{"one", "two", "three"}, []string{"one", "two", "three"}[0:0]))
}

func TestMockedServiceCheck(t *testing.T) {
	sender := NewMockSender("1")
	sender.SetupAcceptAll()

	tags := []string{"one", "two"}
	message := "message 1"
	sender.ServiceCheck("docker.exit", metrics.ServiceCheckOK, "", tags, message)
	sender.AssertCalled(t, "ServiceCheck", "docker.exit", metrics.ServiceCheckOK, "", tags, message)
	sender.AssertNotCalled(t, "ServiceCheck", "docker.exit", metrics.ServiceCheckOK, "", append(tags, "1"), message)
	sender.AssertServiceCheck(t, "docker.exit", metrics.ServiceCheckOK, "", tags, message)
	sender.AssertServiceCheckExclusive(t, "docker.exit", metrics.ServiceCheckOK, "", tags, message)

	tags = append(tags, "a", "b", "c")
	message = "message 2"
	sender.ServiceCheck("docker.exit", metrics.ServiceCheckCritical, "", tags, message)
	sender.AssertCalled(t, "ServiceCheck", "docker.exit", metrics.ServiceCheckCritical, "", tags, message)
	sender.AssertNotCalled(t, "ServiceCheck", "docker.exit", metrics.ServiceCheckCritical, "", append(tags, "1"), message)
	sender.AssertServiceCheck(t, "docker.exit", metrics.ServiceCheckCritical, "", tags, message)
	sender.AssertServiceCheckNotCalled(t, "docker.exit", metrics.ServiceCheckCritical, "", append(tags, "1"), message)
	sender.AssertServiceCheckExclusive(t, "docker.exit", metrics.ServiceCheckCritical, "", tags, message)

	message = "message 3"
	sender.ServiceCheck("docker.exit", metrics.ServiceCheckWarning, "", tags, message)
	sender.AssertCalled(t, "ServiceCheck", "docker.exit", metrics.ServiceCheckWarning, "", tags, message)
	sender.AssertNotCalled(t, "ServiceCheck", "docker.exit", metrics.ServiceCheckWarning, "", append(tags, "1"), message)
	sender.AssertServiceCheck(t, "docker.exit", metrics.ServiceCheckWarning, "", tags, message)
	sender.AssertServiceCheckNotCalled(t, "docker.exit", metrics.ServiceCheckWarning, "", tags, "other message")
	sender.AssertServiceCheckExclusive(t, "docker.exit", metrics.ServiceCheckWarning, "", tags, message)

	message = "message 4"
	tags = append(tags, "container_name:redis")
	sender.ServiceCheck("docker.exit", metrics.ServiceCheckWarning, "", tags, message)
	sender.AssertCalled(t, "ServiceCheck", "docker.exit", metrics.ServiceCheckWarning, "", tags, message)
	sender.AssertNotCalled(t, "ServiceCheck", "docker.exit", metrics.ServiceCheckWarning, "", append(tags, "1"), message)
	sender.AssertServiceCheck(t, "docker.exit", metrics.ServiceCheckWarning, "", tags, message)
	sender.AssertServiceCheckNotCalled(t, "docker.exit", metrics.ServiceCheckWarning, "", tags, "other message")
	sender.AssertServiceCheckExclusive(t, "docker.exit", metrics.ServiceCheckWarning, "", tags, message)
}
