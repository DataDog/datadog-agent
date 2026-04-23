// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"bytes"
	"encoding/json"
	"expvar"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collectorcomp "github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

type mockCollector struct {
	checks []check.Info
}

func (m *mockCollector) RunCheck(_ check.Check) (checkid.ID, error) { return "", nil }
func (m *mockCollector) StopCheck(_ checkid.ID) error               { return nil }
func (m *mockCollector) MapOverChecks(cb func([]check.Info))        { cb(m.checks) }
func (m *mockCollector) GetChecks() []check.Check                   { return nil }
func (m *mockCollector) ReloadAllCheckInstances(_ string, _ []check.Check) ([]checkid.ID, error) {
	return nil, nil
}
func (m *mockCollector) AddEventReceiver(_ collectorcomp.EventReceiver) {}

var (
	testInventoriesMu   sync.RWMutex
	testInventoriesData interface{} = map[string]interface{}{}
)

func TestMain(m *testing.M) {
	// Register the inventories expvar once using expvar.Func, matching how the
	// inventorychecks component publishes it in production (raw JSON object, not a quoted string).
	expvar.Publish("inventories", expvar.Func(func() interface{} {
		testInventoriesMu.RLock()
		defer testInventoriesMu.RUnlock()
		return testInventoriesData
	}))
	os.Exit(m.Run())
}

func setInventories(checkMetadata map[string][]map[string]string) {
	testInventoriesMu.Lock()
	defer testInventoriesMu.Unlock()
	testInventoriesData = map[string]interface{}{"check_metadata": checkMetadata}
}

func clearInventories() {
	testInventoriesMu.Lock()
	defer testInventoriesMu.Unlock()
	testInventoriesData = map[string]interface{}{}
}

func TestRender(t *testing.T) {
	// We're checking that some dates are correctly formatted in the HTML
	// so we need to set the timezone to UTC to avoid issues.
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")
	defer func() {
		os.Setenv("TZ", originalTZ)
	}()

	for _, test := range []struct {
		name        string
		fixtureFile string
		resultFile  string
	}{
		{
			name:        "collectorHTML.tmpl",
			fixtureFile: "fixtures/status_info.json",
			resultFile:  "fixtures/status_info.html",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			jsonBytes, err := os.ReadFile(test.fixtureFile)
			require.NoError(t, err)
			var data map[string]interface{}
			err = json.Unmarshal(jsonBytes, &data)
			require.NoError(t, err)

			output := new(bytes.Buffer)
			err = status.RenderHTML(templatesFS, "collectorHTML.tmpl", output, data)
			require.NoError(t, err, "failed to render HTML")

			expectedOutput, err := os.ReadFile(test.resultFile)
			require.NoError(t, err)

			// We replace windows line break by linux so the tests pass on every OS
			result := strings.ReplaceAll(string(expectedOutput), "\r\n", "\n")
			stringOutput := strings.ReplaceAll(output.String(), "\r\n", "\n")

			require.Equal(t, result, stringOutput, "HTML rendering is not as expected")
		})
	}
}

func TestCollectCheckMetadata_NilCollector(t *testing.T) {
	t.Cleanup(clearInventories)
	setInventories(map[string][]map[string]string{
		"cpu": {
			{"config.hash": "hash1", "version.raw": "1.0.0"},
		},
	})

	p := Provider{coll: nil}
	result := p.collectCheckMetadata()

	require.Contains(t, result, "hash1")
	assert.Equal(t, "1.0.0", result["hash1"]["version.raw"])
}

func TestCollectCheckMetadata_WithCollector(t *testing.T) {
	t.Cleanup(clearInventories)

	coll := &mockCollector{
		checks: []check.Info{
			check.MockInfo{
				Name:    "cpu",
				CheckID: checkid.ID("hash2"),
				Source:  "file:conf.d/cpu.d/conf.yaml",
			},
		},
	}
	p := Provider{coll: coll}
	result := p.collectCheckMetadata()

	require.Contains(t, result, "hash2")
	assert.Equal(t, "conf.d/cpu.d/conf.yaml", result["hash2"]["config.source"])
}

func TestCollectCheckMetadata_ExpvarOverlayAddsFields(t *testing.T) {
	// Expvar-only fields (e.g. version.raw) should be merged in for known hashes.
	setInventories(map[string][]map[string]string{
		"cpu": {
			{"config.hash": "hash3", "version.raw": "2.0.0"},
		},
	})
	t.Cleanup(clearInventories)

	coll := &mockCollector{
		checks: []check.Info{
			check.MockInfo{
				Name:    "cpu",
				CheckID: checkid.ID("hash3"),
				Source:  "file:conf.d/cpu.d/conf.yaml",
			},
		},
	}
	p := Provider{coll: coll}
	result := p.collectCheckMetadata()

	require.Contains(t, result, "hash3")
	assert.Equal(t, "2.0.0", result["hash3"]["version.raw"], "expvar-only field should be overlaid")
	assert.Equal(t, "conf.d/cpu.d/conf.yaml", result["hash3"]["config.source"], "GetMetadata value should be preserved")
}

func TestCollectCheckMetadata_CollectorPrecedenceOverExpvar(t *testing.T) {
	// GetMetadata keys must not be overwritten by expvar values.
	setInventories(map[string][]map[string]string{
		"cpu": {
			{"config.hash": "hash4", "config.source": "expvar-source", "version.raw": "3.0.0"},
		},
	})
	t.Cleanup(clearInventories)

	coll := &mockCollector{
		checks: []check.Info{
			check.MockInfo{
				Name:    "cpu",
				CheckID: checkid.ID("hash4"),
				Source:  "file:collector-source",
			},
		},
	}
	p := Provider{coll: coll}
	result := p.collectCheckMetadata()

	require.Contains(t, result, "hash4")
	assert.Equal(t, "collector-source", result["hash4"]["config.source"], "GetMetadata value must not be overwritten by expvar")
	assert.Equal(t, "3.0.0", result["hash4"]["version.raw"], "expvar-only field should still be added")
}

func TestCollectCheckMetadata_UnknownHashesNotAdded(t *testing.T) {
	// Expvar entries for hashes not in the collector result must not be added.
	setInventories(map[string][]map[string]string{
		"some-check": {
			{"config.hash": "stale-hash", "version.raw": "1.0.0"},
		},
	})
	t.Cleanup(clearInventories)

	coll := &mockCollector{checks: []check.Info{}}
	p := Provider{coll: coll}
	result := p.collectCheckMetadata()

	assert.NotContains(t, result, "stale-hash", "stale hashes from expvar should not appear in collector path")
}
