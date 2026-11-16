// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v3"

	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/externalhost"
	pkgconfigmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
	logs "github.com/DataDog/datadog-agent/pkg/util/log/setup"
	logslog "github.com/DataDog/datadog-agent/pkg/util/log/slog"
	"github.com/DataDog/datadog-agent/pkg/version"
)

import "C"

func testGetVersion(t *testing.T) {
	var v *C.char
	GetVersion(&v)
	require.NotNil(t, v)

	av, _ := version.Agent()
	assert.Equal(t, av.GetNumber(), C.GoString(v))
}

func testGetHostname(t *testing.T) {
	var h *C.char
	GetHostname(&h)
	require.NotNil(t, h)

	hname, _ := hostname.Get(context.Background())
	assert.Equal(t, hname, C.GoString(h))
}

func testGetClusterName(t *testing.T) {
	var ch *C.char
	GetClusterName(&ch)
	require.NotNil(t, ch)

	assert.Equal(t, clustername.GetClusterName(context.Background(), ""), C.GoString(ch))
}

func testHeaders(t *testing.T) {
	var headers *C.char
	Headers(&headers)
	require.NotNil(t, headers)

	h := httpHeaders()
	jsonPayload, _ := json.Marshal(h)
	assert.Equal(t, string(jsonPayload), C.GoString(headers))
}

func testGetConfig(t *testing.T) {
	var config *C.char

	GetConfig(C.CString("does not exist"), &config)
	require.Nil(t, config)

	GetConfig(C.CString("cmd_port"), &config)
	require.NotNil(t, config)
	assert.Equal(t, "5001", C.GoString(config))
}

func testSetExternalTags(t *testing.T) {
	ctags := []*C.char{C.CString("tag1"), C.CString("tag2"), nil}

	SetExternalTags(C.CString("test_hostname"), C.CString("test_source_type"), &ctags[0])

	payload := externalhost.GetPayload()
	require.NotNil(t, payload)

	yamlPayload, _ := yaml.Marshal(payload)
	assert.Equal(t,
		"- - test_hostname\n  - test_source_type:\n        - tag1\n        - tag2\n",
		string(yamlPayload))
}

func testEmitAgentTelemetry(t *testing.T) {
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_metric"), 1.0, C.CString("gauge"))

	// Test second time for laziness check
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_metric"), 1.0, C.CString("gauge"))

	// Test for lock problems
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			time.Sleep(time.Millisecond * time.Duration(rand.IntN(10)))
			EmitAgentTelemetry(C.CString("test_check"), C.CString("test_metric"), 1.0, C.CString("gauge"))
			wg.Done()
		}()
	}
	wg.Wait()

	// Test that changing the metric type doesn't crash the agent for all the permutations
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_metric"), 1.0, C.CString("counter"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_counter"), 1.0, C.CString("histogram"))

	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_counter"), 1.0, C.CString("counter"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_counter"), 1.0, C.CString("counter"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_counter"), 1.0, C.CString("histogram"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_counter"), 1.0, C.CString("gauge"))

	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_histogram"), 1.0, C.CString("histogram"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_histogram"), 1.0, C.CString("histogram"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_histogram"), 1.0, C.CString("counter"))
	EmitAgentTelemetry(C.CString("test_check"), C.CString("test_histogram"), 1.0, C.CString("gauge"))

	assert.True(t, true)
}

func testObfuscaterConfig(t *testing.T) {
	pkgconfigmodel.CleanOverride(t)
	_ = pkgconfigmock.New(t)
	o := lazyInitObfuscator()
	o.Stop()
	expected := obfuscate.Config{
		ES: obfuscate.JSONConfig{
			Enabled:            true,
			KeepValues:         []string{},
			ObfuscateSQLValues: []string{},
		},
		OpenSearch: obfuscate.JSONConfig{
			Enabled:            true,
			KeepValues:         []string{},
			ObfuscateSQLValues: []string{},
		},
		Mongo:                defaultMongoObfuscateSettings,
		SQLExecPlan:          defaultSQLPlanObfuscateSettings,
		SQLExecPlanNormalize: defaultSQLPlanNormalizeSettings,
		HTTP: obfuscate.HTTPConfig{
			RemoveQueryString: false,
			RemovePathDigits:  false,
		},
		Redis: obfuscate.RedisConfig{
			Enabled:       true,
			RemoveAllArgs: false,
		},
		Valkey: obfuscate.ValkeyConfig{
			Enabled:       true,
			RemoveAllArgs: false,
		},
		Memcached: obfuscate.MemcachedConfig{
			Enabled:     true,
			KeepCommand: false,
		},
		CreditCard: obfuscate.CreditCardsConfig{
			Enabled:    true,
			Luhn:       false,
			KeepValues: []string{},
		},
		Cache: obfuscate.CacheConfig{
			Enabled: true,
			MaxSize: 5000000,
		},
	}
	assert.Equal(t, expected, obfuscaterConfig)
}

func testReportIssue(t *testing.T) {
	hp := healthplatformmock.New(t)
	SetHealthPlatform(hp)
	t.Cleanup(func() { SetHealthPlatform(nil) })

	t.Run("partial_json_id_only", func(t *testing.T) {
		var errOut *C.char
		ReportIssue(C.CString("integration:mysql"), C.CString(`{"id":"partial-minimal"}`), &errOut)
		require.Nil(t, errOut, reportIssueErrMsg(errOut))
		got := hp.GetIssue("partial-minimal")
		require.NotNil(t, got)
		assert.Equal(t, "partial-minimal", got.Id)
		assert.Empty(t, got.IssueName, "optional proto fields omitted in JSON should unmarshal as empty")
		assert.Equal(t, "integration:mysql", got.Source, "ReportIssue sets Source from check name")
	})

	t.Run("partial_json_subset_of_fields", func(t *testing.T) {
		var errOut *C.char
		payload := `{"id":"partial-rich","issueName":"conn-timeout","title":"DB timeout","severity":"ISSUE_SEVERITY_MEDIUM"}`
		ReportIssue(C.CString("py:check"), C.CString(payload), &errOut)
		require.Nil(t, errOut, reportIssueErrMsg(errOut))
		got := hp.GetIssue("partial-rich")
		require.NotNil(t, got)
		assert.Equal(t, "partial-rich", got.Id)
		assert.Equal(t, "conn-timeout", got.IssueName)
		assert.Equal(t, "DB timeout", got.Title)
		assert.Equal(t, healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_MEDIUM, got.Severity)
		assert.Equal(t, "py:check", got.Source)
		assert.Empty(t, got.Description)
	})

	t.Run("missing_id_rejected", func(t *testing.T) {
		var errOut *C.char
		ReportIssue(C.CString("c"), C.CString(`{"issueName":"orphan-name"}`), &errOut)
		require.NotNil(t, errOut)
		assert.Contains(t, C.GoString(errOut), "empty or null id")
	})

	t.Run("invalid_json_rejected", func(t *testing.T) {
		var errOut *C.char
		ReportIssue(C.CString("c"), C.CString(`{`), &errOut)
		require.NotNil(t, errOut)
		assert.NotEmpty(t, C.GoString(errOut))
	})
}

func reportIssueErrMsg(errOut *C.char) string {
	if errOut == nil {
		return ""
	}
	return C.GoString(errOut)
}

func testLogMessageWithStack(t *testing.T) {
	var mu sync.Mutex
	var logs []errortracking.ErrorLog
	submit := errortracking.Submitter(func(e errortracking.ErrorLog) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, e)
	})
	bouncer := errortracking.NewBouncer(time.Hour, 0)
	handler := errortracking.NewHandler(func() errortracking.Submitter { return submit }).
		WithBouncerLoader(func() *errortracking.Bouncer { return bouncer })
	log.SetupLogger(logslog.NewWrapper(handler), "debug")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "debug") })

	stack := C.CString("  File \"/opt/datadog-agent/embedded/lib/python3.13/site-packages/datadog_checks/redisdb/redisdb.py\", line 42, in check\n")
	excType := C.CString("redis.exceptions.ConnectionError")
	msg := C.CString("message with user data")

	LogMessageWithStack(msg, 40, stack, excType)

	mu.Lock()
	require.Len(t, logs, 1)
	assert.Equal(t, "<external>.ConnectionError", logs[0].ErrorKind)
	assert.Equal(t, "redisdb", logs[0].Integration)
	assert.True(t, strings.HasPrefix(logs[0].Stack, "datadog_checks.redisdb.redisdb.check\n\tdatadog_checks/redisdb/redisdb.py:42"))
	logs = nil
	mu.Unlock()

	// Without a stack the message is logged like before, with a Go stack.
	LogMessageWithStack(msg, 40, nil, nil)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, logs, 1)
	assert.Empty(t, logs[0].Stack)
	assert.NotZero(t, logs[0].PCsLen)
}

func testLogStackEnabled(t *testing.T) {
	t.Cleanup(func() { logs.RegisterErrortrackingSubmitter(nil) })

	logs.RegisterErrortrackingSubmitter(nil)
	assert.Equal(t, C.int(0), LogStackEnabled(), "no submitter registered: the stack is not wanted")

	logs.RegisterErrortrackingSubmitter(func(errortracking.ErrorLog) {})
	assert.Equal(t, C.int(1), LogStackEnabled())

	logs.RegisterErrortrackingSubmitter(nil)
	assert.Equal(t, C.int(0), LogStackEnabled(), "unregistering takes effect immediately")
}
