// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"context"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/collector/externalhost"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
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

	h := util.HTTPHeaders()
	yamlPayload, _ := yaml.Marshal(h)
	assert.Equal(t, string(yamlPayload), C.GoString(headers))
}

func testGetConfig(t *testing.T) {
	var config *C.char

	GetConfig(C.CString("does not exist"), &config)
	require.Nil(t, config)

	GetConfig(C.CString("cmd_port"), &config)
	require.NotNil(t, config)
	assert.Equal(t, "5001\n", C.GoString(config))
}

func testSetExternalTags(t *testing.T) {
	ctags := []*C.char{C.CString("tag1"), C.CString("tag2"), nil}

	SetExternalTags(C.CString("test_hostname"), C.CString("test_source_type"), &ctags[0])

	payload := externalhost.GetPayload()
	require.NotNil(t, payload)

	yamlPayload, _ := yaml.Marshal(payload)
	assert.Equal(t,
		"- - test_hostname\n  - test_source_type:\n    - tag1\n    - tag2\n",
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
	conf := pkgconfigmodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	pkgconfigsetup.InitConfig(conf)
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
		},
	}
	assert.Equal(t, expected, obfuscaterConfig)
}
