// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"io"
	"net/http"
	"testing"

	gopsutilhost "github.com/shirou/gopsutil/v3/host"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	telemetrypkg "github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// HTTP client mock
type clientMock struct {
	body []byte
}

func (c *clientMock) Do(req *http.Request) (*http.Response, error) {
	c.body, _ = io.ReadAll(req.Body)
	return &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
	}, nil
}

func newClientMock() client {
	return &clientMock{}
}

// Runner mock (TODO: use use mock.Mock)
type runnerMock struct {
	mock.Mock
	jobs []job
}

func (r *runnerMock) run() {
	for _, j := range r.jobs {
		j.a.run(j.profiles)
	}
}

func (r *runnerMock) start() {
}

func (r *runnerMock) stop() context.Context {
	return context.Background()
}

func (r *runnerMock) addJob(j job) {
	r.jobs = append(r.jobs, j)
}

func newRunnerMock() runner {
	return &runnerMock{}
}

// Host component currently does not have a mock
type hostMock struct {
}

func (s hostMock) GetPayloadAsJSON(context.Context) ([]byte, error) {
	return []byte{}, nil
}
func (s hostMock) GetInformation() *gopsutilhost.InfoStat {
	return &gopsutilhost.InfoStat{}
}
func newHostMock() hostMock {
	return hostMock{}
}

// Status component currently has mock but it appears to be not compatible with fx  fx fails
type statusMock struct {
}

func (s statusMock) GetStatus(string, bool, ...string) ([]byte, error) {
	return []byte{}, nil
}
func (s statusMock) GetStatusBySection(string, string, bool) ([]byte, error) {
	return []byte{}, nil
}
func newStatusMock() statusMock {
	return statusMock{}
}

// aggregator mock function
func getTestAtel(t *testing.T,
	tel telemetry.Component,
	confOverrides map[string]any,
	client client,
	runner runner) *atel {

	cfg := fxutil.Test[config.Component](t, config.MockModule(),
		fx.Replace(config.MockParams{Overrides: confOverrides}))
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	status := fxutil.Test[status.Component](t,
		func() fxutil.Module {
			return fxutil.Component(
				fx.Provide(newStatusMock),
				fx.Provide(func(s statusMock) status.Component { return s }))
		}())
	host := fxutil.Test[host.Component](t,
		func() fxutil.Module {
			return fxutil.Component(
				fx.Provide(newHostMock),
				fx.Provide(func(m hostMock) host.Component { return m }))
		}())

	sndr, err := newSenderImpl(cfg, log, host, client)
	assert.NoError(t, err)

	return createAtel(cfg, log, tel, status, host, sndr, runner)
}

func TestEnabled(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, telemetry.MockModule())
	override := map[string]any{
		"agent_telemetry.enabled": true,
	}
	client := newClientMock()
	runner := newRunnerMock()

	// setup and initiate atel
	a := getTestAtel(t, tel, override, client, runner)

	assert.True(t, a.enabled)
}

func TestRun(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, telemetry.MockModule())
	override := map[string]any{
		"agent_telemetry.enabled": true,
	}
	client := newClientMock()
	runner := newRunnerMock()

	// setup and initiate atel
	a := getTestAtel(t, tel, override, client, runner)
	a.start()

	// default configuration has 1 job with 2 profiles (more configurations needs to be tested)
	// will be improved in future by providing deterministic configuration
	assert.Equal(t, 1, len(runner.(*runnerMock).jobs))
	assert.Equal(t, 2, len(runner.(*runnerMock).jobs[0].profiles))
}

func TestTelemetryReportMetricBasic(t *testing.T) {
	// Little hack. Telemetry component is not fully componentized, and relies on global registry so far
	// so we need to reset it before running the test. This is not ideal and will be improved in the future.
	// TODO: moved Status and Metric collection to an interface and use a mock for testing
	tel := fxutil.Test[telemetry.Mock](t, telemetry.MockModule())
	tel.Reset()
	counter := telemetrypkg.NewCounter("checks", "execution_time", []string{"check_name"}, "")
	counter.Inc("mycheck")
	override := map[string]any{
		"agent_telemetry.enabled": true,
	}
	client := newClientMock()
	runner := newRunnerMock()

	// setup and initiate atel
	a := getTestAtel(t, tel, override, client, runner)
	a.start()

	// run the runner to trigger the telemetry report
	runner.(*runnerMock).run()

	assert.True(t, len(client.(*clientMock).body) > 0)

	// for more sophisticated comparison, we could unmarshal the body and check the content
	// will use jsondiff.CompareJSON(src, dest, jsondiff.Equivalent())	"github.com/wI2L/jsondiff")
}
