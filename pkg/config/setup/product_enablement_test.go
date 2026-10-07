// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestProductEnablementSettingsDefaults(t *testing.T) {
	conf := newTestConf(t)

	assert.Equal(t, "", conf.GetString("sku"))
	assert.Empty(t, conf.GetStringSlice("products"))
}

func TestProductEnablementSettingsFromEnv(t *testing.T) {
	t.Setenv("DD_SKU", "sku_a")
	t.Setenv("DD_PRODUCTS", "product_a,product_b product_c")
	conf := newTestConf(t)

	assert.Equal(t, "sku_a", conf.GetString("sku"))
	assert.Equal(t, []string{"product_a", "product_b", "product_c"}, conf.GetStringSlice("products"))
}

var (
	testSKUDefinitions = map[string][]string{
		"sku_a": {"product_a"},
		"sku_b": {"product_d", "product_e"},
	}
	// product_a -> product_b, product_c ; product_b -> product_c, product_d
	testProductDependencies = map[string][]string{
		"product_a": {"product_b", "product_c"},
		"product_b": {"product_c", "product_d"},
		"product_c": {},
		"product_d": {},
		"product_e": {},
		"product_f": {"product_g"},
		"product_g": {"product_f"}, // cycle: must terminate
	}
)

func TestResolveProducts(t *testing.T) {
	tests := []struct {
		name     string
		sku      string
		products []string
		expected []string
	}{
		{name: "nothing enabled", expected: []string{}},
		{name: "sku only", sku: "sku_b", expected: []string{"product_d", "product_e"}},
		{name: "products only", products: []string{"product_e"}, expected: []string{"product_e"}},
		{name: "transitive dependencies", products: []string{"product_a"}, expected: []string{"product_a", "product_b", "product_c", "product_d"}},
		{name: "sku and products", sku: "sku_a", products: []string{"product_e"}, expected: []string{"product_a", "product_b", "product_c", "product_d", "product_e"}},
		{name: "duplicates", sku: "sku_b", products: []string{"product_d", "product_d"}, expected: []string{"product_d", "product_e"}},
		{name: "cycle", products: []string{"product_f"}, expected: []string{"product_f", "product_g"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := resolveProducts(test.sku, test.products, testSKUDefinitions, testProductDependencies)
			require.NoError(t, err)
			assert.Equal(t, test.expected, resolved)
		})
	}
}

func TestResolveProductsErrors(t *testing.T) {
	_, err := resolveProducts("unknown_sku", nil, testSKUDefinitions, testProductDependencies)
	assert.ErrorContains(t, err, "unknown SKU 'unknown_sku'")

	_, err = resolveProducts("", []string{"product_a", "unknown_product"}, testSKUDefinitions, testProductDependencies)
	assert.ErrorContains(t, err, "unknown product 'unknown_product'")

	_, err = resolveProducts("", []string{"sku_a"}, testSKUDefinitions, testProductDependencies)
	assert.ErrorContains(t, err, "'sku_a' is a SKU")
}

func parseTestSchema(t *testing.T, content string) map[string]interface{} {
	t.Helper()
	var root map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(content), &root))
	return root
}

func TestCollectProductDefaults(t *testing.T) {
	root := parseTestSchema(t, `
properties:
  logs_enabled:
    node_type: setting
    default: false
    product_defaults:
      product_a: true
      product_b: true
  disabled_product_only:
    node_type: setting
    default: 1
    product_defaults:
      product_z: 2
  logs_config:
    node_type: section
    properties:
      tags:
        node_type: setting
        default: []
        product_defaults:
          product_a: [a, b]
          product_b: [a, b]
      nested:
        node_type: section
        properties:
          mapping:
            node_type: setting
            default: {}
            product_defaults:
              product_a: {x: 1, y: 2}
              product_b: {y: 2, x: 1}
`)

	defaults, err := collectProductDefaults(root, []string{"product_a", "product_b"})
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"logs_enabled":               true,
		"logs_config.tags":           []interface{}{"a", "b"},
		"logs_config.nested.mapping": map[string]interface{}{"x": 1, "y": 2},
	}, defaults)
}

func TestCollectProductDefaultsPlatform(t *testing.T) {
	root := parseTestSchema(t, `
properties:
  setting:
    node_type: setting
    default: none
    product_platform_defaults:
      product_a:
        fargate: fargate_value
        container: container_value
        `+runtime.GOOS+`: os_value
        other: other_value
  only_other:
    node_type: setting
    default: none
    product_platform_defaults:
      product_a:
        other: other_value
  no_match:
    node_type: setting
    default: none
    product_platform_defaults:
      product_a:
        not_this_os: value
`)
	enabled := []string{"product_a"}

	t.Setenv("DOCKER_DD_AGENT", "")
	t.Setenv("ECS_FARGATE", "")
	t.Setenv("AWS_EXECUTION_ENV", "")
	defaults, err := collectProductDefaults(root, enabled)
	require.NoError(t, err)
	// a platform without value means the product doesn't change the setting
	assert.Equal(t, map[string]interface{}{"setting": "os_value", "only_other": "other_value"}, defaults)

	t.Setenv("DOCKER_DD_AGENT", "true")
	defaults, err = collectProductDefaults(root, enabled)
	require.NoError(t, err)
	assert.Equal(t, "container_value", defaults["setting"])

	t.Setenv("ECS_FARGATE", "true")
	defaults, err = collectProductDefaults(root, enabled)
	require.NoError(t, err)
	assert.Equal(t, "fargate_value", defaults["setting"])
}

func TestGetPlatformDefaultKubernetes(t *testing.T) {
	values := map[string]interface{}{
		"fargate":    "fargate_value",
		"kubernetes": "kubernetes_value",
		"container":  "container_value",
		"other":      "other_value",
	}
	clearEnv := func() {
		for _, name := range []string{"ECS_FARGATE", "AWS_EXECUTION_ENV", "KUBERNETES_SERVICE_PORT", "KUBERNETES", "DOCKER_DD_AGENT"} {
			t.Setenv(name, "")
		}
	}

	clearEnv()
	t.Setenv("DOCKER_DD_AGENT", "true")
	assert.Equal(t, "container_value", getPlatformDefault(values))

	// kubernetes is more specific than container
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	assert.Equal(t, "kubernetes_value", getPlatformDefault(values))

	// fargate is more specific than kubernetes
	t.Setenv("ECS_FARGATE", "true")
	assert.Equal(t, "fargate_value", getPlatformDefault(values))

	// kubernetes falls back to container, then to the OS and 'other'
	clearEnv()
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	t.Setenv("DOCKER_DD_AGENT", "true")
	assert.Equal(t, "container_value", getPlatformDefault(map[string]interface{}{"container": "container_value", "other": "other_value"}))
	assert.Equal(t, "other_value", getPlatformDefault(map[string]interface{}{"other": "other_value"}))
}

func TestCollectProductDefaultsConflicts(t *testing.T) {
	root := parseTestSchema(t, `
properties:
  logs_enabled:
    node_type: setting
    default: false
    product_defaults:
      product_a: true
      product_b: false
      product_c: true
  agreeing:
    node_type: setting
    default: 0
    product_defaults:
      product_a: 1
      product_b: 1
  section:
    node_type: section
    properties:
      list:
        node_type: setting
        default: []
        product_defaults:
          product_a: [a, b]
          product_b: [b, a]
`)

	_, err := collectProductDefaults(root, []string{"product_a", "product_b", "product_c"})
	require.Error(t, err)
	// every conflict is reported, in a deterministic order
	assert.Equal(t, "the enabled products set conflicting values:\n"+
		"  - logs_enabled: product_a=true, product_b=false, product_c=true\n"+
		"  - section.list: product_a=[a b], product_b=[b a]",
		err.Error())
}

const testCoreSchema = `
sku_definitions:
  sku_a: [product_a]
product_dependencies:
  product_a: [product_b]
  product_b: []
  product_c: []
properties:
  logs_enabled:
    node_type: setting
    default: false
    product_defaults:
      product_a: true
      product_c: false
  forwarder_timeout:
    node_type: setting
    default: 20
    product_defaults:
      product_b: 42
  cluster_checks:
    node_type: section
    properties:
      rebalance_period:
        node_type: setting
        default: 10m0s
        product_defaults:
          product_b: 30s
`

func testSchemaGetter(content string) SchemaGetter {
	return func() ([]byte, error) { return []byte(content), nil }
}

// useSchemaGetters replaces the schemas used by product enablement for the duration of the test
func useSchemaGetters(t *testing.T, core, systemProbe SchemaGetter) {
	previousCore, previousSystemProbe := coreSchemaGetter, systemProbeSchemaGetter
	coreSchemaGetter, systemProbeSchemaGetter = core, systemProbe
	t.Cleanup(func() { coreSchemaGetter, systemProbeSchemaGetter = previousCore, previousSystemProbe })
}

// useTestSchemas uses testCoreSchema and testSystemProbeSchema for the duration of the test
func useTestSchemas(t *testing.T) {
	useSchemaGetters(t, testSchemaGetter(testCoreSchema), testSchemaGetter(testSystemProbeSchema))
}

// captureLogs redirects the logs to a buffer for the duration of the test
func captureLogs(t *testing.T) *bytes.Buffer {
	var buffer bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndMsgFormat(&buffer, log.InfoLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "info")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "info") })
	return &buffer
}

func TestApplyProductEnablementNothingEnabled(t *testing.T) {
	conf := newTestConf(t)
	getter := func() ([]byte, error) {
		t.Fatal("the schema must not be loaded when no product is enabled")
		return nil, nil
	}

	useSchemaGetters(t, getter, getter)
	require.NoError(t, ApplyProductEnablement(conf))
	assert.Equal(t, map[string]interface{}{}, conf.AllSettingsBySource()[pkgconfigmodel.SourceProductEnablement])
	assert.Empty(t, ResolvedProducts())
}

func TestApplyProductEnablement(t *testing.T) {
	logs := captureLogs(t)
	conf := confFromYAML(t, "sku: sku_a")

	useTestSchemas(t)
	require.NoError(t, ApplyProductEnablement(conf))

	assert.True(t, conf.GetBool("logs_enabled"))
	assert.Equal(t, pkgconfigmodel.SourceProductEnablement, conf.GetSource("logs_enabled"))
	assert.Equal(t, 42, conf.GetInt("forwarder_timeout"))
	assert.Equal(t, 30*time.Second, conf.GetDuration("cluster_checks.rebalance_period"))
	assert.Equal(t, []string{"product_a", "product_b"}, ResolvedProducts())
	assert.Contains(t, logs.String(), "Product enablement: sku=sku_a products=[] resolved=[product_a product_b]")
}

func TestApplyProductEnablementUserConfigWins(t *testing.T) {
	conf := confFromYAML(t, "products: [product_a]\nlogs_enabled: false")

	useTestSchemas(t)
	require.NoError(t, ApplyProductEnablement(conf))

	assert.False(t, conf.GetBool("logs_enabled"))
	assert.Equal(t, pkgconfigmodel.SourceFile, conf.GetSource("logs_enabled"))
	assert.Equal(t, true, conf.AllSettingsBySource()[pkgconfigmodel.SourceProductEnablement].(map[string]interface{})["logs_enabled"])
}

func TestApplyProductEnablementErrors(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		getter   SchemaGetter
		expected string
	}{
		{name: "conflict", yaml: "products: [product_a, product_c]", getter: testSchemaGetter(testCoreSchema), expected: "logs_enabled: product_a=true, product_c=false"},
		{name: "unknown sku", yaml: "sku: unknown", getter: testSchemaGetter(testCoreSchema), expected: "unknown SKU 'unknown'"},
		{name: "invalid schema", yaml: "sku: sku_a", getter: testSchemaGetter("{"), expected: "could not load the schema"},
		{name: "schema getter error", yaml: "sku: sku_a", getter: func() ([]byte, error) { return nil, errors.New("no schema") }, expected: "no schema"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conf := confFromYAML(t, test.yaml)

			useSchemaGetters(t, test.getter, test.getter)
			err := ApplyProductEnablement(conf)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.expected)
			// nothing is applied on error
			assert.Equal(t, map[string]interface{}{}, conf.AllSettingsBySource()[pkgconfigmodel.SourceProductEnablement])
			assert.Empty(t, ResolvedProducts())
		})
	}
}

const testSystemProbeSchema = `
properties:
  network_config:
    node_type: section
    properties:
      enabled:
        node_type: setting
        default: false
        product_defaults:
          product_b: true
  system_probe_config:
    node_type: section
    properties:
      max_conns_per_message:
        node_type: setting
        default: 600
        product_defaults:
          product_a: 100
          product_c: 200
`

func newTestSystemProbeConf(t *testing.T) pkgconfigmodel.BuildableConfig {
	conf := newEmptyMockConf(t)
	InitSystemProbeConfig(conf)
	return conf
}

func TestApplySystemProbeProductEnablement(t *testing.T) {
	coreConf := confFromYAML(t, "sku: sku_a")
	spConf := newTestSystemProbeConf(t)

	useTestSchemas(t)
	require.NoError(t, ApplySystemProbeProductEnablement(spConf, coreConf))

	// sku_a enables product_a and its dependency product_b
	assert.True(t, spConf.GetBool("network_config.enabled"))
	assert.Equal(t, pkgconfigmodel.SourceProductEnablement, spConf.GetSource("network_config.enabled"))
	assert.Equal(t, 100, spConf.GetInt("system_probe_config.max_conns_per_message"))
	// the core configuration is left untouched
	assert.Equal(t, map[string]interface{}{}, coreConf.AllSettingsBySource()[pkgconfigmodel.SourceProductEnablement])
}

func TestApplySystemProbeProductEnablementNothingEnabled(t *testing.T) {
	coreConf := confFromYAML(t, "{}")
	spConf := newTestSystemProbeConf(t)
	getter := func() ([]byte, error) {
		t.Fatal("the schemas must not be loaded when no product is enabled")
		return nil, nil
	}

	useSchemaGetters(t, getter, getter)
	require.NoError(t, ApplySystemProbeProductEnablement(spConf, coreConf))
	assert.Equal(t, map[string]interface{}{}, spConf.AllSettingsBySource()[pkgconfigmodel.SourceProductEnablement])
}

func TestApplySystemProbeProductEnablementConflict(t *testing.T) {
	coreConf := confFromYAML(t, "products: [product_a, product_c]")
	spConf := newTestSystemProbeConf(t)

	// the core schema conflicts too (logs_enabled) but only the system-probe settings are applied here
	useTestSchemas(t)
	err := ApplySystemProbeProductEnablement(spConf, coreConf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "system_probe_config.max_conns_per_message: product_a=100, product_c=200")
	assert.NotContains(t, err.Error(), "logs_enabled")
	assert.Equal(t, map[string]interface{}{}, spConf.AllSettingsBySource()[pkgconfigmodel.SourceProductEnablement])
}

func TestApplySystemProbeProductEnablementCoreNotLoaded(t *testing.T) {
	previous := isTestBinary
	isTestBinary = func() bool { return false }
	t.Cleanup(func() { isTestBinary = previous })

	coreConf := newTestConf(t) // the configuration sources were never read
	spConf := newTestSystemProbeConf(t)

	useTestSchemas(t)
	err := ApplySystemProbeProductEnablement(spConf, coreConf)
	assert.ErrorContains(t, err, "the core configuration must be loaded")
}

// Products are applied while loading the configuration, before the override funcs: the override funcs (ex: the
// infrastructure mode overrides) see the values set by products.
func TestLoadDatadogAppliesProducts(t *testing.T) {
	useSchemaGetters(t, testSchemaGetter(`
product_dependencies:
  eudm: []
properties:
  infrastructure_mode:
    node_type: setting
    default: full
    product_defaults:
      eudm: end_user_device
`), testSchemaGetter(testSystemProbeSchema))
	conf := newTestConf(t)
	configPath := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("products: [eudm]"), 0o600))
	conf.SetConfigFile(configPath)
	// The override funcs are global and other tests clean them: register one that records what it sees
	var modeSeenByOverrides string
	pkgconfigmodel.AddOverrideFunc(func(c pkgconfigmodel.Config) { modeSeenByOverrides = c.GetString("infrastructure_mode") })
	pkgconfigmodel.CleanOverride(t)

	require.NoError(t, LoadDatadog(conf, secretsmock.New(t), delegatedauthmock.New(t), nil))

	assert.Equal(t, "end_user_device", modeSeenByOverrides)
	assert.Equal(t, "end_user_device", conf.GetString("infrastructure_mode"))
	assert.Equal(t, pkgconfigmodel.SourceProductEnablement, conf.GetSource("infrastructure_mode"))
	assert.Equal(t, []string{"eudm"}, ResolvedProducts())
}

// Environment variables are loaded even without configuration file: products must be applied too
func TestLoadDatadogAppliesProductsWithoutConfigFile(t *testing.T) {
	useTestSchemas(t)
	t.Setenv("DD_PRODUCTS", "product_b")
	conf := newTestConf(t)
	conf.SetConfigFile(filepath.Join(t.TempDir(), "missing.yaml"))

	err := LoadDatadog(conf, secretsmock.New(t), delegatedauthmock.New(t), nil)
	assert.ErrorIs(t, err, pkgconfigmodel.ErrConfigFileNotFound)
	assert.NotErrorIs(t, err, ErrProductEnablement)
	assert.Equal(t, 42, conf.GetInt("forwarder_timeout"))
}

func TestLoadDatadogProductError(t *testing.T) {
	useTestSchemas(t)
	conf := newTestConf(t)
	configPath := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("products: [product_a, product_c]\nforwarder_timeout: 7"), 0o600))
	conf.SetConfigFile(configPath)

	err := LoadDatadog(conf, secretsmock.New(t), delegatedauthmock.New(t), nil)
	require.ErrorIs(t, err, ErrProductEnablement)
	assert.Contains(t, err.Error(), "logs_enabled: product_a=true, product_c=false")
	// the rest of the configuration is loaded
	assert.Equal(t, 7, conf.GetInt("forwarder_timeout"))

	loadErr, productErr := SplitProductEnablementError(err)
	assert.NoError(t, loadErr)
	assert.ErrorIs(t, productErr, ErrProductEnablement)
}

func TestSplitProductEnablementError(t *testing.T) {
	productErr := fmt.Errorf("%w: conflict", ErrProductEnablement)
	loadErr := errors.New("load error")

	gotLoad, gotProduct := SplitProductEnablementError(nil)
	assert.NoError(t, gotLoad)
	assert.NoError(t, gotProduct)

	gotLoad, gotProduct = SplitProductEnablementError(loadErr)
	assert.Equal(t, loadErr, gotLoad)
	assert.NoError(t, gotProduct)

	gotLoad, gotProduct = SplitProductEnablementError(productErr)
	assert.NoError(t, gotLoad)
	assert.Equal(t, productErr, gotProduct)

	gotLoad, gotProduct = SplitProductEnablementError(withProductEnablementError(loadErr, productErr))
	assert.Equal(t, loadErr, gotLoad)
	assert.Equal(t, productErr, gotProduct)
}

func TestLoadSystemProbeAppliesProducts(t *testing.T) {
	useTestSchemas(t)
	coreConf := confFromYAML(t, "products: [product_b]")
	previous := GlobalConfigBuilder()
	SetDatadog(coreConf)                       // nolint: forbidigo // the system-probe configuration reads the products of the global core configuration
	t.Cleanup(func() { SetDatadog(previous) }) // nolint: forbidigo // restore the global configuration
	spConf := newTestSystemProbeConf(t)
	spConf.SetConfigFile(filepath.Join(t.TempDir(), "missing.yaml"))

	err := LoadSystemProbe(spConf, nil)
	assert.ErrorIs(t, err, pkgconfigmodel.ErrConfigFileNotFound)
	assert.True(t, spConf.GetBool("network_config.enabled"))
}
