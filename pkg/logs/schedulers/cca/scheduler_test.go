package cca

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	logsConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func setup() (scheduler *Scheduler, ac *autodiscovery.AutoConfig, spy *schedulers.MockSourceManager) {
	ac = &autodiscovery.AutoConfig{}
	scheduler = New(func() *autodiscovery.AutoConfig { return ac }).(*Scheduler)
	spy = &schedulers.MockSourceManager{}
	return
}

func TestNothingWhenNoConfig(t *testing.T) {
	scheduler, _, spy := setup()
	config := coreConfig.Mock()
	config.Set("logs_config.container_collect_all", false)

	scheduler.Start(spy)

	require.Equal(t, 0, len(spy.Events))
}

func TestAfterACStarts(t *testing.T) {
	scheduler, ac, spy := setup()
	config := coreConfig.Mock()
	config.Set("logs_config.container_collect_all", true)

	scheduler.Start(spy)

	// nothing added yet
	require.Equal(t, 0, len(spy.Events))

	// Fake autoconfig running..
	ac.ForceRanOnceFlag()

	// wait for the source to be added
	<-scheduler.added

	source := spy.Events[0].Source
	assert.Equal(t, "container_collect_all", source.Name)
	assert.Equal(t, logsConfig.DockerType, source.Config.Type)
	assert.Equal(t, "docker", source.Config.Source)
	assert.Equal(t, "docker", source.Config.Service)
}
