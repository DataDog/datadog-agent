// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeAIUsageTaskXML(t *testing.T) {
	out, err := encodeAIUsageTaskXML("Ab")
	require.NoError(t, err)
	// BOM (FF FE) followed by 'A' (41 00) and 'b' (62 00), little-endian.
	require.Equal(t, []byte{0xFF, 0xFE, 0x41, 0x00, 0x62, 0x00}, out)
}

func TestReadAIUsageChromeExtensionIDFromFile(t *testing.T) {
	dir := t.TempDir()

	withID := filepath.Join(dir, "with_id.yaml")
	require.NoError(t, os.WriteFile(withID, []byte("chrome_extension_id: \"  abc123  \"\n"), 0o644))
	assert.Equal(t, "abc123", readAIUsageChromeExtensionIDFromFile(withID))

	withoutID := filepath.Join(dir, "without_id.yaml")
	require.NoError(t, os.WriteFile(withoutID, []byte("trace_agent_url: \"http://127.0.0.1:8126\"\n"), 0o644))
	assert.Equal(t, "", readAIUsageChromeExtensionIDFromFile(withoutID))

	assert.Equal(t, "", readAIUsageChromeExtensionIDFromFile(filepath.Join(dir, "missing.yaml")))
}

func TestWriteAIUsageManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, aiUsageNativeHostName+".json")

	// An obsolete manifest in the same dir should be cleaned up.
	obsolete := filepath.Join(dir, aiUsageObsoleteNativeHostName+".json")
	require.NoError(t, os.WriteFile(obsolete, []byte("{}"), 0o644))

	require.NoError(t, writeAIUsageManifest(manifestPath, `C:\Program Files\Datadog\host.exe`, "extid"))

	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	manifest := string(data)
	assert.Contains(t, manifest, `"name": "`+aiUsageNativeHostName+`"`)
	assert.Contains(t, manifest, `"path": "C:\\Program Files\\Datadog\\host.exe"`)
	assert.Contains(t, manifest, `"chrome-extension://extid/"`)
	assert.NoFileExists(t, obsolete)
}

func TestWriteAIUsageConfigSubstitutesTraceURLAndPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	examplePath := filepath.Join(dir, aiUsageConfigName+".example")
	configPath := filepath.Join(dir, aiUsageConfigName)
	require.NoError(t, os.WriteFile(examplePath, []byte("# comment\ntrace_agent_url: \"http://127.0.0.1:8126\"\nevp_proxy_api_version: 2\n"), 0o644))

	require.NoError(t, writeAIUsageConfig(examplePath, configPath))
	rendered, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(rendered), "trace_agent_url: ")
	assert.Contains(t, string(rendered), "evp_proxy_api_version: 2")

	// An existing config must be preserved.
	require.NoError(t, os.WriteFile(configPath, []byte("preserved"), 0o644))
	require.NoError(t, writeAIUsageConfig(examplePath, configPath))
	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, "preserved", string(after))
}

func TestBuildAIUsageTaskXML(t *testing.T) {
	xml, err := buildAIUsageTaskXML(`C:\Program Files\Datadog\host.exe`, `C:\ProgramData\Datadog\ai_usage_native_host.yaml`)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(xml, `<?xml version="1.0" encoding="UTF-16"?>`))
	assert.Contains(t, xml, "<LogonTrigger>")
	assert.Contains(t, xml, "<GroupId>"+aiUsageUsersGroupSID+"</GroupId>")
	assert.Contains(t, xml, "<RunLevel>LeastPrivilege</RunLevel>")
	assert.Contains(t, xml, "<Command>C:\\Program Files\\Datadog\\host.exe</Command>")
	assert.Contains(t, xml, "--desktop-monitor --config &#34;C:\\ProgramData\\Datadog\\ai_usage_native_host.yaml&#34;")
}
