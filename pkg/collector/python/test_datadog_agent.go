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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/collector/externalhost"
	pkgconfigmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
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

func testLoadSQLConfig(t *testing.T) {
	optStr := `{"dbms":"postgresql","obfuscation_mode":"obfuscate_and_normalize","table_names":true,"return_json_metadata":true}`

	c1, err := loadSQLConfig(optStr)
	require.NoError(t, err)
	require.NotNil(t, c1)
	assert.Equal(t, obfuscate.ObfuscateAndNormalize, c1.ObfuscationMode)
	assert.Equal(t, "postgresql", c1.DBMS)
	assert.True(t, c1.TableNames)
	assert.True(t, c1.ReturnJSONMetadata)

	c2, err := loadSQLConfig(optStr)
	require.NoError(t, err)
	assert.Same(t, c1, c2, "identical options string should return the memoized *sqlConfig")

	// memoized parse must equal a fresh json.Unmarshal (behavior preserved)
	var fresh sqlConfig
	require.NoError(t, json.Unmarshal([]byte(optStr), &fresh))
	assert.Equal(t, fresh, *c1)

	// a different options string gets its own config (no key collision)
	mc, err := loadSQLConfig(`{"dbms":"mysql","obfuscation_mode":"obfuscate_only"}`)
	require.NoError(t, err)
	assert.Equal(t, "mysql", mc.DBMS)
	assert.Equal(t, obfuscate.ObfuscateOnly, mc.ObfuscationMode)
	assert.NotSame(t, c1, mc)
	again, _ := loadSQLConfig(optStr)
	assert.Equal(t, "postgresql", again.DBMS)

	badStr := `{not valid json`
	bad, err := loadSQLConfig(badStr)
	require.Error(t, err)
	require.NotNil(t, bad)
	_, cached := sqlConfigCache.Load(badStr)
	assert.False(t, cached, "a failed parse must not be cached")
}

func testObfuscateSQL(t *testing.T) {
	pkgconfigmodel.CleanOverride(t)
	_ = pkgconfigmock.New(t)
	// use a fresh, non-stopped obfuscator regardless of test ordering
	obfuscator = obfuscate.NewObfuscator(obfuscate.Config{})
	obfuscatorLoader.Do(func() {})

	query := "SELECT * FROM users WHERE id = 123 AND name = 'alice'"
	opts := `{"dbms":"postgresql","obfuscation_mode":"obfuscate_and_normalize","table_names":true,"return_json_metadata":true}`

	var errMsg *C.char
	res := C.GoString(ObfuscateSQL(C.CString(query), C.CString(opts), &errMsg))
	require.Nil(t, errMsg)

	var oq struct {
		Query    string `json:"query"`
		Metadata struct {
			TablesCSV string `json:"tables_csv"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal([]byte(res), &oq))
	assert.NotContains(t, oq.Query, "123")
	assert.NotContains(t, oq.Query, "alice")
	assert.Contains(t, oq.Query, "?")
	assert.Contains(t, oq.Metadata.TablesCSV, "users")

	// same query+options again (memo + cache) -> byte-identical output
	errMsg = nil
	res2 := C.GoString(ObfuscateSQL(C.CString(query), C.CString(opts), &errMsg))
	require.Nil(t, errMsg)
	assert.Equal(t, res, res2)

	// return_json_metadata:false -> bare obfuscated SQL, not JSON
	errMsg = nil
	bare := C.GoString(ObfuscateSQL(C.CString(query), C.CString(`{"dbms":"postgresql","obfuscation_mode":"obfuscate_and_normalize"}`), &errMsg))
	require.Nil(t, errMsg)
	assert.NotContains(t, bare, "123")
	assert.Error(t, json.Unmarshal([]byte(bare), &oq), "bare result should be raw SQL, not JSON")

	// empty options -> defaults to "{}", must not error or panic
	errMsg = nil
	assert.NotEmpty(t, C.GoString(ObfuscateSQL(C.CString(query), C.CString(""), &errMsg)))
	require.Nil(t, errMsg)
}
