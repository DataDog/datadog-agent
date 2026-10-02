// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib && zstd && (darwin || windows)

package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/version"
	"go.yaml.in/yaml/v3"
)

// This opt-in audit checks a real artifact on its source device. It does not
// prove service continuity, backend delivery, or capture failure isolation.
// Inputs are JSON arrays of absolute paths, never credentials in environment
// values. Configuration and token contents remain in this process's memory.
func TestCaptureArtifactPrivacyAudit(t *testing.T) {
	directory := os.Getenv("EUDM_CAPTURE_BUNDLE")
	if directory == "" {
		t.Skip("set EUDM_CAPTURE_BUNDLE to a complete capture from this device")
	}
	defer func() {
		if recover() != nil {
			t.Fatal("artifact audit failed while decoding typed samples")
		}
	}()
	loaded, err := bundle.Load(directory, version.FullCommit)
	if err != nil {
		t.Fatal("capture bundle provenance, completeness, or typed validation failed")
	}
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	if loaded.Manifest.Profile.OS != platform {
		t.Fatal("artifact privacy audit must run on the captured platform")
	}
	var identities []string
	host, err := os.Hostname()
	if err != nil {
		t.Fatal("cannot obtain hostname for privacy check")
	}
	identities = append(identities, host)
	current, err := user.Current()
	if err != nil {
		t.Fatal("cannot obtain user for privacy check")
	}
	identities = append(identities, current.Username, current.HomeDir)
	if invoking := os.Getenv("SUDO_USER"); invoking != "" {
		original, err := user.Lookup(invoking)
		if err != nil {
			t.Fatal("cannot obtain invoking user for privacy check")
		}
		identities = append(identities, original.Username, original.HomeDir)
	}
	system, err := checks.CollectSystemInfo()
	if err != nil {
		t.Fatal("cannot obtain native system identity for privacy check")
	}
	identities = append(identities, system.Uuid)
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal("cannot obtain interfaces for privacy check")
	}
	for _, iface := range interfaces {
		identities = append(identities, iface.HardwareAddr.String())
		addresses, err := iface.Addrs()
		if err != nil {
			t.Fatal("cannot obtain interface addresses for privacy check")
		}
		for _, addr := range addresses {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && !ip.IsUnspecified() {
				identities = append(identities, ip.String())
			}
		}
	}

	credentials, unresolved, err := readAuditCredentials(auditInputPaths(t, "EUDM_AUDIT_CONFIG_FILES"), auditInputPaths(t, "EUDM_AUDIT_TOKEN_FILES"))
	if err != nil {
		t.Fatal("cannot read explicit credential audit inputs")
	}
	identities = append(identities, credentials...)
	check := func(data []byte) {
		t.Helper()
		if auditSensitiveContent(data, identities) {
			t.Fatal("native identity or credential found in persisted capture content")
		}
	}
	auditBundleSamples(t, loaded, check)
	if unresolved {
		t.Fatal("credential audit incomplete: secret-backend resolved values were unavailable; no secret backend was executed")
	}
	t.Log("typed sample validation, complete groups, native identity scans, and explicit credential scans passed")
}

// Inspect JSON values, including nested Gohai JSON, rather than mistaking a
// normal field named "root" for the invoking username.
func auditSensitiveContent(data []byte, identities []string) bool {
	scan := func(data []byte) bool {
		lower := bytes.ToLower(data)
		for _, identity := range identities {
			escaped, _ := json.Marshal(identity)
			if identity != "" && (bytes.Contains(lower, bytes.ToLower([]byte(identity))) || bytes.Contains(lower, bytes.ToLower(escaped[1:len(escaped)-1]))) {
				return true
			}
		}
		return false
	}
	var visit func(any) bool
	visit = func(value any) bool {
		switch value := value.(type) {
		case map[string]any:
			for _, child := range value {
				if visit(child) {
					return true
				}
			}
		case []any:
			for _, child := range value {
				if visit(child) {
					return true
				}
			}
		case string:
			var nested any
			trimmed := strings.TrimSpace(value)
			if (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) && json.Unmarshal([]byte(value), &nested) == nil {
				return visit(nested)
			}
			return scan([]byte(value))
		}
		return false
	}
	var decoded any
	if json.Unmarshal(data, &decoded) == nil {
		return visit(decoded)
	}
	return scan(data)
}

func auditInputPaths(t *testing.T, name string) []string {
	t.Helper()
	var paths []string
	if json.Unmarshal([]byte(os.Getenv(name)), &paths) != nil || len(paths) == 0 {
		t.Fatal("explicit config and token file lists are required for credential auditing")
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			t.Fatal("credential audit inputs require absolute paths")
		}
	}
	return paths
}

func readAuditCredentials(configPaths, tokenPaths []string) ([]string, bool, error) {
	invalid := errors.New("cannot read credential audit inputs")
	read := func(path string) ([]byte, error) {
		file, err := os.Open(path)
		if err != nil {
			return nil, invalid
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
		if err != nil || len(data) > 4<<20 {
			return nil, invalid
		}
		return data, nil
	}
	var values []string
	unresolved := false
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if strings.HasPrefix(value, "ENC[") && strings.HasSuffix(value, "]") {
			unresolved = true
			return
		}
		values = append(values, value)
	}
	for _, path := range tokenPaths {
		data, err := read(path)
		if err != nil || len(bytes.TrimSpace(data)) == 0 {
			return nil, false, invalid
		}
		add(string(data))
	}
	configStart := len(values)
	for _, path := range configPaths {
		data, err := read(path)
		if err != nil {
			return nil, false, invalid
		}
		var root yaml.Node
		if yaml.Unmarshal(data, &root) != nil {
			return nil, false, invalid
		}
		var walk func(*yaml.Node, bool)
		type visit struct {
			node     *yaml.Node
			selected bool
		}
		seen := map[visit]bool{}
		walk = func(node *yaml.Node, selected bool) {
			key := visit{node, selected}
			if node == nil || seen[key] {
				return
			}
			seen[key] = true
			switch node.Kind {
			case yaml.AliasNode:
				walk(node.Alias, selected)
			case yaml.MappingNode:
				for i := 0; i+1 < len(node.Content); i += 2 {
					key := strings.ToLower(node.Content[i].Value)
					sensitive := selected || slices.Contains([]string{"api_key", "app_key", "application_key", "password", "token", "auth_token", "client_secret", "additional_endpoints"}, key) || strings.HasSuffix(key, "_api_key")
					walk(node.Content[i+1], sensitive)
				}
			case yaml.ScalarNode:
				if selected && node.Tag == "!!str" {
					add(node.Value)
				}
			default:
				for _, child := range node.Content {
					walk(child, selected)
				}
			}
		}
		walk(&root, false)
	}
	if len(values) == configStart && !unresolved {
		return nil, unresolved, invalid
	}
	return values, unresolved, nil
}

func auditBundleSamples(t *testing.T, loaded *bundle.Loaded, check func([]byte)) {
	t.Helper()
	manifest, err := json.Marshal(loaded.Manifest)
	if err != nil {
		t.Fatal("cannot inspect capture manifest")
	}
	check(manifest)
	for _, ref := range loaded.Manifest.Samples {
		check(loaded.Files[ref.File])
	}
}

func TestArtifactTypedFixtures(t *testing.T) {
	for _, platform := range []string{"macos", "windows"} {
		t.Run(platform, func(t *testing.T) {
			fixture := replayFixture(t, platform)
			for _, sample := range fixture.Samples {
				if sample.Connections == nil {
					continue
				}
				if config := sample.Connections.AgentConfiguration; config == nil || !config.EudmEnabled || config.NpmEnabled != (platform == "windows") {
					t.Fatal("typed connection fixture lost the native product-routing flags")
				}
			}
			auditBundleSamples(t, fixture, func(data []byte) {
				if auditSensitiveContent(data, []string{"synthetic-audit-credential"}) {
					t.Fatal("credential escaped typed capture")
				}
			})
		})
	}
}

func TestArtifactSensitiveContentScansValues(t *testing.T) {
	for _, test := range []struct {
		body string
		want bool
	}{
		{`{"root":"/capture","user":"user-1234"}`, false},
		{`{"user":"root"}`, true},
		{`{"user":"\u0072oot"}`, true},
		{`{"gohai":"{\"hostname\":\"synthetic-secret\"}"}`, true},
		{"\x08\x00synthetic-secret\xff", true},
	} {
		if auditSensitiveContent([]byte(test.body), []string{"root", "synthetic-secret"}) != test.want {
			t.Fatal("privacy scan confused JSON field names or missed decoded values")
		}
	}
}

func TestArtifactCredentialInputsStayInMemory(t *testing.T) {
	directory := t.TempDir()
	configPath, tokenPath := filepath.Join(directory, "config.yaml"), filepath.Join(directory, "auth_token")
	config := "common: &credential synthetic-primary-key\napi_key: *credential\nadditional_endpoints:\n  https://synthetic.invalid:\n    - synthetic-extra-key\nproxy:\n  password: synthetic-proxy-secret\napp_key: ENC[unavailable-backend-key]\n"
	if os.WriteFile(configPath, []byte(config), 0600) != nil || os.WriteFile(tokenPath, []byte("synthetic-ipc-token\n"), 0600) != nil {
		t.Fatal("cannot construct synthetic audit inputs")
	}
	values, unresolved, err := readAuditCredentials([]string{configPath}, []string{tokenPath})
	if err != nil || !unresolved || len(values) != 4 {
		t.Fatal("explicit credential collection lost values or secret-backend limitation")
	}
	for _, value := range []string{"synthetic-primary-key", "synthetic-extra-key", "synthetic-proxy-secret", "synthetic-ipc-token"} {
		if !slices.Contains(values, value) {
			t.Fatal("explicit credential was not collected for in-memory scanning")
		}
	}
}
