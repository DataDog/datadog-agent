// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorychecksimpl

import (
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	collector "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	collectormock "github.com/DataDog/datadog-agent/comp/collector/collector/mock"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	logsBundle "github.com/DataDog/datadog-agent/comp/logs"
	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logagent "github.com/DataDog/datadog-agent/comp/logs/agent/def"
	logagentmock "github.com/DataDog/datadog-agent/comp/logs/agent/mock"
	inventoryagentmock "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// testDeps bridges fx injection for tests since Requires has no fx.In marker.
type testDeps struct {
	fx.In

	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	Coll       option.Option[collector.Component]
	LogAgent   option.Option[logagent.Component]
	Hostname   hostnameinterface.Component
}

func makeRequires(deps testDeps) Requires {
	return Requires{
		Log:        deps.Log,
		Config:     deps.Config,
		Serializer: deps.Serializer,
		Coll:       deps.Coll,
		LogAgent:   deps.LogAgent,
		Hostname:   deps.Hostname,
	}
}

func getTestInventoryChecks(t *testing.T, coll option.Option[collector.Component], logAgent option.Option[logagent.Component], overrides map[string]any) *inventorychecksImpl {
	p := NewComponent(
		makeRequires(fxutil.Test[testDeps](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMockWithOverrides(t, overrides) }),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
			fx.Provide(func() option.Option[collector.Component] {
				return coll
			}),
			fx.Provide(func() option.Option[logagent.Component] {
				return logAgent
			}),
			hostnameimpl.MockModule(),
		)),
	)
	return p.Comp.(*inventorychecksImpl)
}

func TestSet(t *testing.T) {
	ic := getTestInventoryChecks(
		t, option.None[collector.Component](), option.Option[logagent.Component]{}, nil,
	)

	ic.Set("instance_1", "key", "value")

	assert.Len(t, ic.data, 1)
	assert.Contains(t, ic.data, "instance_1")
	assert.Contains(t, ic.data["instance_1"].metadata, "key")
	assert.Equal(t, ic.data["instance_1"].metadata["key"], "value")

	ic.Set("instance_1", "key2", "value2")

	assert.Len(t, ic.data, 1)
	assert.Contains(t, ic.data["instance_1"].metadata, "key2")
	assert.Equal(t, ic.data["instance_1"].metadata["key2"], "value2")
}

func TestSetEmptyInstance(t *testing.T) {
	ic := getTestInventoryChecks(
		t, option.None[collector.Component](), option.Option[logagent.Component]{}, nil,
	)

	ic.Set("", "key", "value")

	assert.Len(t, ic.data, 0)
}

func TestGetInstanceMetadata(t *testing.T) {
	ic := getTestInventoryChecks(
		t, option.None[collector.Component](), option.Option[logagent.Component]{}, nil,
	)

	ic.Set("instance_1", "key1", "value1")
	ic.Set("instance_1", "key2", "value2")

	res := ic.GetInstanceMetadata("instance_1")
	assert.Equal(t,
		map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
		res,
	)

	assert.Empty(t, ic.GetInstanceMetadata("instance_2"))
}

func TestGetPayload(t *testing.T) {
	for _, invChecksCfgEnabled := range []bool{true, false} {
		cInfo := []check.Info{
			check.MockInfo{
				Name:         "check1",
				CheckID:      checkid.ID("check1_instance1"),
				Source:       "provider1:/etc/datadog-agent/conf.d/redis.yaml[0]",
				InitConf:     "",
				InstanceConf: "{\"test\":21}",
			},
			check.MockInfo{
				Name:         "check1",
				CheckID:      checkid.ID("check1_instance2"),
				Source:       "provider1:/etc/datadog-agent/conf.d/redis.yaml[1]",
				InitConf:     "",
				InstanceConf: "{\"test\":22}",
			},
			check.MockInfo{
				Name:         "check2",
				CheckID:      checkid.ID("check2_instance1"),
				Source:       "provider2:/etc/datadog-agent/conf.d/redis2.yaml[0]",
				InitConf:     "{}",
				InstanceConf: "{}",
			},
		}

		mockColl := fxutil.Test[collector.Component](t,
			fx.Replace(collectormock.MockParams{
				ChecksInfo: cInfo,
			}),
			collectormock.MockModule(),
			core.MockBundle(),
			hostnameimpl.MockModule(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		)

		// Setup log sources
		logSources := sources.NewLogSources()
		src := sources.NewLogSource("redisdb", &logConfig.LogsConfig{
			Type:       logConfig.FileType,
			Path:       "/var/log/redis/redis.log",
			Identifier: "redisdb",
			Service:    "awesome_cache",
			Source:     "redis",
			Tags:       []string{"env:prod"},
		})
		// Register an error
		src.Status().Error(errors.New("No such file or directory"))
		logSources.AddSource(src)
		fakeTagger := taggerfxmock.SetupFakeTagger(t)

		mockLogAgent := fxutil.Test[option.Option[logagentmock.Mock]](
			t,
			logsBundle.MockBundle(),
			core.MockBundle(),
			secretsnoopfx.Module(),
			hostnameimpl.MockModule(),
			inventoryagentmock.MockModule(),
			logscompression.MockModule(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			fx.Provide(func() tagger.Component {
				return fakeTagger
			}),
		)
		logsAgent, _ := mockLogAgent.Get()
		logsAgent.SetSources(logSources)

		testName := fmt.Sprintf("inventories_checks_configuration_enabled=%t", invChecksCfgEnabled)
		t.Run(testName, func(t *testing.T) {
			overrides := map[string]any{
				"inventories_configuration_enabled":        true,
				"inventories_checks_configuration_enabled": invChecksCfgEnabled,
			}

			ic := getTestInventoryChecks(t,
				option.New[collector.Component](mockColl),
				option.New[logagent.Component](logsAgent),
				overrides,
			)

			ic.hostname = "test-hostname"

			ic.Set("check1_instance1", "check_provided_key1", 123)
			ic.Set("check1_instance1", "check_provided_key2", "Hi")
			ic.Set("non_running_checkid", "check_provided_key1", "this_should_not_be_kept")

			p := ic.getPayloadWithConfigs().(*Payload)

			assert.Equal(t, "test-hostname", p.Hostname)

			// TODO: non-determinism around jmx state caused by it being a global object
			assert.True(t, len(p.Metadata) >= 2 && len(p.Metadata) <= 3) // 'non_running_checkid' should have been cleaned
			assert.Len(t, p.Metadata["check1"], 2)                       // check1 has two instances

			check1Instance1 := p.Metadata["check1"][0]
			assert.Equal(t, "check1_instance1", check1Instance1["config.hash"])
			assert.Equal(t, "provider1", check1Instance1["config.provider"])
			assert.Equal(t, "/etc/datadog-agent/conf.d/redis.yaml[0]", check1Instance1["config.source"])
			assert.Equal(t, 123, check1Instance1["check_provided_key1"])
			assert.Equal(t, "Hi", check1Instance1["check_provided_key2"])
			if invChecksCfgEnabled {
				assert.Equal(t, "", check1Instance1["init_config"])
				assert.Equal(t, "test: 21", check1Instance1["instance_config"])
			} else {
				assert.Nil(t, check1Instance1["init_config"])
				assert.Nil(t, check1Instance1["instance_config"])
			}

			check1Instance2 := p.Metadata["check1"][1]
			assert.Equal(t, "check1_instance2", check1Instance2["config.hash"])
			assert.Equal(t, "provider1", check1Instance2["config.provider"])
			assert.Equal(t, "/etc/datadog-agent/conf.d/redis.yaml[1]", check1Instance2["config.source"])
			if invChecksCfgEnabled {
				assert.Equal(t, "", check1Instance2["init_config"])
				assert.Equal(t, "test: 22", check1Instance2["instance_config"])
			} else {
				assert.Nil(t, check1Instance2["init_config"])
				assert.Nil(t, check1Instance2["instance_config"])
			}

			assert.Len(t, p.Metadata["check2"], 1) // check2 has one instance
			check2Instance1 := p.Metadata["check2"][0]
			assert.Equal(t, "check2_instance1", check2Instance1["config.hash"])
			assert.Equal(t, "provider2", check2Instance1["config.provider"])
			assert.Equal(t, "/etc/datadog-agent/conf.d/redis2.yaml[0]", check2Instance1["config.source"])
			if invChecksCfgEnabled {
				assert.Equal(t, "{}", check2Instance1["init_config"])
				assert.Equal(t, "{}", check2Instance1["instance_config"])
			} else {
				assert.Nil(t, check2Instance1["init_config"])
				assert.Nil(t, check2Instance1["instance_config"])
			}

			// Check that metadata linked to non-existing check were deleted
			assert.NotContains(t, "non_running_checkid", ic.data)

			// Check the log sources part of the metadata
			if invChecksCfgEnabled {
				assert.Len(t, p.LogsMetadata, 1)
				actualSource, found := p.LogsMetadata["redisdb"]
				assert.True(t, found)
				assert.Len(t, actualSource, 1)
				expectedSourceConfig := `{"type":"file","path":"/var/log/redis/redis.log","service":"awesome_cache","source":"redis","tags":["env:prod"]}`
				assert.Equal(t, expectedSourceConfig, actualSource[0]["config"])
				expectedSourceStatus := map[string]string{
					"status": "error",
					"error":  "Error: No such file or directory",
				}
				assert.Equal(t, expectedSourceStatus, actualSource[0]["state"])
				assert.Equal(t, "awesome_cache", actualSource[0]["service"])
				assert.Equal(t, "redis", actualSource[0]["source"])
				assert.Equal(t, logConfig.StringSliceField{"env:prod"}, actualSource[0]["tags"])
			} else {
				assert.Len(t, p.LogsMetadata, 0)
			}
		})
	}
}

func TestGetPayloadFilesMetadata(t *testing.T) {
	confdPath := t.TempDir()
	files := []string{filepath.Join(confdPath, "http_check.d", "one.yaml"), filepath.Join(confdPath, "http_check.d", "two.yaml")}
	require.NoError(t, os.MkdirAll(filepath.Dir(files[0]), 0700))
	for _, file := range files {
		require.NoError(t, os.WriteFile(file, []byte(`init_config: {password: 'ENC[shared]'}
instances: [{custom_field: some_value, password: 'ENC[shared]'}, {password: raw-value}]
logs: [{password: raw-value}]
jmx_metrics: [{password: 'ENC[shared]'}]
`), 0600))
	}
	ic := getTestInventoryChecks(t, option.None[collector.Component](), option.Option[logagent.Component]{}, nil)
	providers.ResetReader([]string{confdPath})
	t.Cleanup(func() { providers.ResetReader(nil) })
	payload := ic.getPayload(true).(*Payload)
	require.Len(t, payload.FilesMetadata, 2)
	for _, file := range files {
		require.Contains(t, payload.FilesMetadata, file)
		assert.Contains(t, payload.FilesMetadata[file], "raw_config")
		encoded, err := json.Marshal(payload.FilesMetadata[file])
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "raw-value")
		var decoded struct{ Secrets []map[string]string }
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		assert.ElementsMatch(t, []map[string]string{
			{"path": "init_config.password", "kind": "enc", "handle": "shared"},
			{"path": "instances.0.password", "kind": "enc", "handle": "shared"},
			{"path": "instances.1.password", "kind": "raw"},
			{"path": "logs.0.password", "kind": "raw"},
			{"path": "jmx_metrics.0.password", "kind": "enc", "handle": "shared"},
		}, decoded.Secrets)
	}
	assert.Empty(t, ic.getPayload(false).(*Payload).FilesMetadata)
	ic.conf.Set("metadata_skip_secrets", true, pkgconfigmodel.SourceAgentRuntime)
	for _, file := range ic.getPayload(true).(*Payload).FilesMetadata {
		assert.NotContains(t, file, "secrets")
	}
}

func TestFlareProviderFilename(t *testing.T) {
	ic := getTestInventoryChecks(
		t, option.None[collector.Component](), option.Option[logagent.Component]{}, nil,
	)
	assert.Equal(t, "checks.json", ic.FlareFileName)
}

// TODO (Component): This test will be removed when the inventorychecks component will be move into the collector component
func TestExpvarExist(t *testing.T) {
	getTestInventoryChecks(
		t, option.None[collector.Component](), option.Option[logagent.Component]{}, nil,
	)
	assert.NotNil(t, expvar.Get("inventories"))
}

// Each file declares its own handle even though both configure the same integration.
func secretFileFixture(t *testing.T) (*inventorychecksImpl, *secretsmock.Mock, []string) {
	t.Helper()
	dir := t.TempDir()
	files := []string{filepath.Join(dir, "redisdb.d", "one.yaml"), filepath.Join(dir, "redisdb.d", "two.yaml")}
	require.NoError(t, os.MkdirAll(filepath.Dir(files[0]), 0700))
	resolver := secretsmock.New(t)
	var paths, handles []string
	for i, file := range files {
		handle := fmt.Sprintf("redis_password_%d", i)
		content := fmt.Sprintf("init_config: {}\ninstances:\n- password: ENC[%s]\n", handle)
		require.NoError(t, os.WriteFile(file, []byte(content), 0600))
		conf, _, err := providers.GetIntegrationConfigFromFile("redisdb", file)
		require.NoError(t, err)
		paths = append(paths, conf.Digest()+":password")
		handles = append(handles, handle)
	}
	paths = append(paths, "api_key", "other-origin:password")
	handles = append(handles, "core_key", "other_password")
	resolver.SetPathsAndHandles(paths, handles)
	resolver.SetSecrets(map[string]string{
		"redis_password_0": "first-secret",
		"redis_password_1": "second-secret",
		"core_key":         "core-secret",
		"other_password":   "unrelated-secret",
	})
	providers.ResetReader([]string{dir})
	t.Cleanup(func() { providers.ResetReader(nil) })
	ic := getTestInventoryChecks(t, option.None[collector.Component](), option.None[logagent.Component](), nil)
	ic.secrets = resolver
	return ic, resolver, files
}

func TestEncInfo(t *testing.T) {
	ic, resolver, files := secretFileFixture(t)
	payload := ic.getPayload(true).(*Payload)
	require.Len(t, payload.FilesMetadata, 2)
	for i, file := range files {
		handle := fmt.Sprintf("redis_password_%d", i)
		assert.Equal(t, metadata{
			"paths":          []string{"instances.0.password"},
			"handles":        []string{handle},
			"hash_secret_id": resolver.GetHashValues([]string{handle}),
		}, payload.FilesMetadata[file].(metadata)["enc_info"])
	}
	raw, err := payload.MarshalJSON()
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.NotContains(t, decoded, "enc_info")
	assert.NotContains(t, string(raw), "core_key")
	assert.NotContains(t, string(raw), "other_password")
	assert.Empty(t, ic.getPayload(false).(*Payload).FilesMetadata)

	ic.conf.Set("metadata_skip_secrets", true, pkgconfigmodel.SourceAgentRuntime)
	for _, file := range ic.getPayload(true).(*Payload).FilesMetadata {
		assert.NotContains(t, file, "enc_info")
	}
	ic.conf.Set("metadata_skip_secrets", false, pkgconfigmodel.SourceAgentRuntime)
	ic.secrets = nil
	for _, file := range ic.getPayload(true).(*Payload).FilesMetadata {
		assert.NotContains(t, file, "enc_info")
	}
	ic.secrets = resolver
	// File declarations are authoritative; resolver paths are not needed.
	resolver.SetPathsAndHandles(nil, nil)
	for _, file := range ic.getPayload(true).(*Payload).FilesMetadata {
		assert.Contains(t, file, "enc_info")
	}
	resolver.SetSecrets(nil)
	for _, file := range ic.getPayload(true).(*Payload).FilesMetadata {
		info := file.(metadata)["enc_info"].(metadata)
		assert.Empty(t, info["hash_secret_id"])
	}
}

func TestDiagnosticEndpointScrubsSecretHashes(t *testing.T) {
	ic, resolver, files := secretFileFixture(t)
	hash := resolver.GetHashValues([]string{"redis_password_0"})["redis_password_0"]
	require.NotEmpty(t, hash)
	rec := httptest.NewRecorder()
	ic.writePayloadAsJSON(rec, httptest.NewRequest(http.MethodGet, "/metadata/inventory-checks", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), hash)
	assert.NotContains(t, rec.Body.String(), "first-secret")
	var diagnostic map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &diagnostic))
	file := diagnostic["files_metadata"].(map[string]any)[files[0]].(map[string]any)
	info := file["enc_info"].(map[string]any)
	assert.Equal(t, "********", info["hash_secret_id"])
	assert.Equal(t, []any{"redis_password_0"}, info["handles"])
	backend, err := ic.getPayloadWithConfigs().MarshalJSON()
	require.NoError(t, err)
	assert.Contains(t, string(backend), hash)
}

func TestFileEncInfoUsesDeclarations(t *testing.T) {
	ic, resolver, files := secretFileFixture(t)
	require.NoError(t, os.WriteFile(files[0], []byte("init_config: {password: 'ENC[redis_password_0]'}\ninstances: [{password: 'ENC[redis_password_0]'}, {password: raw-value}]\n"), 0600))
	providers.ResetReader([]string{filepath.Dir(filepath.Dir(files[0]))})
	info := ic.getPayload(true).(*Payload).FilesMetadata[files[0]].(metadata)["enc_info"].(metadata)
	assert.Equal(t, []string{"init_config.password", "instances.0.password"}, info["paths"])
	assert.Equal(t, []string{"redis_password_0", "redis_password_0"}, info["handles"])
	assert.Equal(t, resolver.GetHashValues([]string{"redis_password_0"}), info["hash_secret_id"])

	require.NoError(t, os.WriteFile(files[0], []byte("instances: [{password: raw-value}]\n"), 0600))
	providers.ResetReader([]string{filepath.Dir(filepath.Dir(files[0]))})
	file := ic.getPayload(true).(*Payload).FilesMetadata[files[0]].(metadata)
	assert.NotContains(t, file, "enc_info")
	assert.Contains(t, file, "secrets")
}
