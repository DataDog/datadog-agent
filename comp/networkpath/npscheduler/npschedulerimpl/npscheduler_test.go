package npschedulerimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder/forwarderimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// testOptions is a fx collection of common dependencies for all tests
var testOptions = fx.Options(
	Module(),
	forwarderimpl.MockModule(),
	demultiplexerimpl.MockModule(),
	defaultforwarder.MockModule(),
	core.MockBundle(),
	eventplatformimpl.MockModule(),
)

func TestStartServerAndStopNpScheduler(t *testing.T) {
	var component npscheduler.Component
	app := fxtest.New(t, fx.Options(
		testOptions,
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		fx.Replace(sysprobeconfigimpl.MockParams{Overrides: map[string]any{
			"network_path.enabled": true,
		}}),
		fx.Populate(&component),
	))
	server := component.(*npSchedulerImpl)
	assert.NotNil(t, server)
	assert.NotNil(t, app)
	assert.False(t, server.running)
	app.RequireStart()
	assert.True(t, server.running)
	app.RequireStop()
	assert.False(t, server.running)
}
