// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	logConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	serverlessLogs "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-agent/pkg/serverless/registration"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestMain(m *testing.M) {
	origShutdownDelay := ShutdownDelay
	ShutdownDelay = 0
	defer func() { ShutdownDelay = origShutdownDelay }()
	os.Exit(m.Run())
}

func TestWaitForDaemonBlocking(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	d.TellDaemonRuntimeStarted()

	complete := false
	go func() {
		<-time.After(10 * time.Millisecond)
		complete = true
		d.TellDaemonRuntimeDone()
	}()
	d.WaitForDaemon()
	assert.Equal(complete, true, "daemon didn't block until TellDaemonRuntimeDone")
}

func GetValueSyncOnce(so *sync.Once) uint64 {
	return reflect.ValueOf(so).Elem().FieldByName("done").Uint()
}

func TestTellDaemonRuntimeDoneOnceStartOnly(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	d.TellDaemonRuntimeStarted()
	assert.Equal(uint64(0), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestTellDaemonRuntimeDoneOnceStartAndEnd(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	d.TellDaemonRuntimeStarted()
	d.TellDaemonRuntimeDone()

	assert.Equal(uint64(1), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestTellDaemonRuntimeDoneIfLocalTest(t *testing.T) {
	t.Setenv(LocalTestEnvVar, "1")
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()
	d.TellDaemonRuntimeStarted()
	client := &http.Client{Timeout: 1 * time.Second}
	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/lambda/flush", port), nil)
	assert.Nil(err)
	response, err := client.Do(request) //nolint:bodyclose
	if err != nil {
		// retry once in case the daemon wasn't ready to accept requests
		time.Sleep(100 * time.Millisecond)
		response, err = client.Do(request) //nolint:bodyclose
	}
	defer response.Body.Close()
	assert.Nil(err)
	select {
	case <-wrapWait(d.RuntimeWg):
		// all good
	case <-time.NewTimer(500 * time.Millisecond).C:
		t.Fail()
	}
}

func wrapWait(wg *sync.WaitGroup) <-chan struct{} {
	out := make(chan struct{})
	go func() {
		wg.Wait()
		out <- struct{}{}
	}()
	return out
}

func TestTellDaemonRuntimeNotDoneIf(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()
	d.TellDaemonRuntimeStarted()
	assert.Equal(uint64(0), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestTellDaemonRuntimeDoneOnceStartAndEndAndTimeout(t *testing.T) {
	assert := assert.New(t)
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	d.TellDaemonRuntimeStarted()
	d.TellDaemonRuntimeDone()
	d.TellDaemonRuntimeDone()

	assert.Equal(uint64(1), GetValueSyncOnce(d.TellDaemonRuntimeDoneOnce))
}

func TestRaceTellDaemonRuntimeStartedVersusTellDaemonRuntimeDone(t *testing.T) {
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	go d.TellDaemonRuntimeStarted()
	go d.TellDaemonRuntimeDone()
}

func TestSetTraceTagNoop(t *testing.T) {
	tagsMap := map[string]string{
		"key0": "value0",
	}
	d := Daemon{
		TraceAgent: nil,
	}
	assert.False(t, d.setTraceTags(tagsMap))
}

func TestSetTraceTagNoopTraceGetNil(t *testing.T) {
	tagsMap := map[string]string{
		"key0": "value0",
	}
	d := Daemon{
		TraceAgent: &trace.ServerlessTraceAgent{},
	}
	assert.False(t, d.setTraceTags(tagsMap))
}

func TestSetTraceTagOk(t *testing.T) {
	tagsMap := map[string]string{
		"key0": "value0",
	}
	var agent = &trace.ServerlessTraceAgent{}
	t.Setenv("DD_API_KEY", "x")
	agent.Start(true, &trace.LoadConfig{Path: "/does-not-exist.yml"}, make(chan *pb.Span), random.Random.Uint64())
	defer agent.Stop()
	d := Daemon{
		TraceAgent: agent,
	}
	assert.True(t, d.setTraceTags(tagsMap))
}

func TestOutOfOrderInvocations(t *testing.T) {
	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	assert.NotPanics(t, d.TellDaemonRuntimeDone)
	d.TellDaemonRuntimeStarted()
}

func TestLogsAreSent(t *testing.T) {
	config.DetectFeatures()
	logsEndpointHasBeenCalled := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	tsLogsIntake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logsEndpointHasBeenCalled = true
		w.WriteHeader(200)
	}))

	t.Setenv("DD_LOGS_CONFIG_LOGS_DD_URL", strings.Replace(tsLogsIntake.URL, "http://", "", 1))
	t.Setenv("DD_LOGS_CONFIG_LOGS_NO_SSL", "true")

	port := testutil.FreeTCPPort(t)
	d := StartDaemon(fmt.Sprint("127.0.0.1:", port))
	defer d.Stop()

	logChannel := make(chan *logConfig.ChannelMessage)
	initDurationChan := make(chan float64)

	metricAgent := &metrics.ServerlessMetricAgent{}
	metricAgent.Start(5*time.Second, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	d.SetStatsdServer(metricAgent)

	d.SetupLogCollectionHandler("/lambda/logs", logChannel, config.Datadog.GetBool("serverless.logs_enabled"), config.Datadog.GetBool("enhanced_metrics"), initDurationChan)
	d.StartLogCollection()

	logRegistrationError := registration.EnableTelemetryCollection(
		registration.EnableTelemetryCollectionArgs{
			ID:                  "myId",
			RegistrationURL:     ts.URL,
			RegistrationTimeout: 10 * time.Second,
			LogsType:            "all",
			Port:                port,
			CollectionRoute:     "/lambda/logs",
			Timeout:             1000,
			MaxBytes:            1000,
			MaxItems:            1000,
		})

	if logRegistrationError != nil {
		log.Error("Can't subscribe to logs:", logRegistrationError)
	} else {
		serverlessLogs.SetupLogAgent(logChannel, "AWS Logs", "lambda", d.LogSyncOrchestrator)
	}

	client := &http.Client{}
	raw, err := os.ReadFile("./valid_logs_payload_1000.json")
	assert.Nil(t, err)
	body := bytes.NewBuffer(raw)
	request, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/lambda/logs", port), body)
	assert.Nil(t, err)
	response, err := client.Do(request)
	assert.Nil(t, err)
	assert.True(t, response.StatusCode == 200)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	d.flushLogs(context.TODO(), wg)
	wg.Wait()
	assert.True(t, logsEndpointHasBeenCalled)
	assert.Equal(t, d.LogSyncOrchestrator.NbMessageSent.Load(), d.LogSyncOrchestrator.TelemetryApiMessageReceivedCount.Load())
}
