package discovery

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDiscovery(t *testing.T) {
	testChan := make(chan snmpJob, 10)

	worker = func(l *Discovery, jobs <-chan snmpJob) {
		for {
			job := <-jobs
			testChan <- job
		}
	}

	checkConfig := &checkconfig.CheckConfig{
		Network:           "192.168.0.0/24",
		CommunityString:   "public",
		DiscoveryInterval: 3600,
		DiscoveryWorkers:  1,
	}
	discovery := NewDiscovery(checkConfig)
	discovery.Start()

	job := <-testChan

	assert.Equal(t, "public", job.subnet.config.CommunityString)
	assert.Equal(t, "192.168.0.0", job.currentIP.String())
	assert.Equal(t, "192.168.0.0", job.subnet.startingIP.String())
	assert.Equal(t, "192.168.0.0/24", job.subnet.config.Network)

	job = <-testChan
	assert.Equal(t, "192.168.0.1", job.currentIP.String())
	assert.Equal(t, "192.168.0.0", job.subnet.startingIP.String())
}

func TestDiscovery_IgnoreAddresses(t *testing.T) {
	testChan := make(chan snmpJob, 10)

	worker = func(l *Discovery, jobs <-chan snmpJob) {
		for {
			job := <-jobs
			testChan <- job
		}
	}

	checkConfig := &checkconfig.CheckConfig{
		Network:            "192.168.0.0/24",
		CommunityString:    "public",
		DiscoveryInterval:  3600,
		DiscoveryWorkers:   1,
		IgnoredIPAddresses: map[string]bool{"192.168.0.0": true},
	}
	discovery := NewDiscovery(checkConfig)
	discovery.Start()

	job := <-testChan

	assert.Equal(t, "public", job.subnet.config.CommunityString)
	assert.Equal(t, "192.168.0.1", job.currentIP.String())
	assert.Equal(t, "192.168.0.0", job.subnet.startingIP.String())

	job = <-testChan
	assert.Equal(t, "192.168.0.2", job.currentIP.String())
	assert.Equal(t, "192.168.0.0", job.subnet.startingIP.String())
}
