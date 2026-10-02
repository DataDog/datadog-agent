// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib && zstd && (darwin || windows)

package integration

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/output"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/tagset"
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
			t.Fatal("artifact audit failed while decoding recorded evidence")
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
	auditBundleEvidence(t, loaded, check)
	if unresolved {
		t.Fatal("credential audit incomplete: secret-backend resolved values were unavailable; no secret backend was executed")
	}
	t.Log("artifact provenance, complete groups, all decoded wire comparisons, native identities, and explicit credential scans passed")
}

// Inspect JSON values, including nested Gohai JSON, rather than mistaking a
// normal field named "root" for the invoking username. Binary protobuf strings
// are checked before conversion, as are their independently decoded semantics.
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

func auditBundleEvidence(t *testing.T, loaded *bundle.Loaded, check func([]byte)) {
	t.Helper()
	manifest, err := json.Marshal(loaded.Manifest)
	if err != nil {
		t.Fatal("cannot inspect capture manifest")
	}
	check(manifest)
	for _, ref := range loaded.Manifest.Samples {
		typed := loaded.Files[ref.File]
		sample := loaded.Samples[ref.File]
		check(typed)
		var wires []bundle.WireReference
		for _, name := range ref.WireFiles {
			var wire bundle.WireReference
			if bundle.DecodeJSON(loaded.Files[name], &wire) != nil {
				t.Fatal("cannot decode capture wire reference")
			}
			header, err := json.Marshal(wire.Headers)
			if err != nil {
				t.Fatal("cannot inspect wire headers")
			}
			check(header)
			if wire.Headers.Get("Authorization") != "" || wire.Headers.Get("DD-API-KEY") != "" {
				t.Fatal("credential-bearing wire header")
			}
			wires = append(wires, wire)
		}
		if sample.Metrics != nil {
			auditMetricEvidence(t, sample.Metrics, ref, wires, check)
			continue
		}
		if sample.Inventory != nil {
			if len(ref.Routes) == 0 {
				t.Fatal("inventory lacks observed routing evidence")
			}
			for _, route := range ref.Routes {
				if route.Endpoint != "/api/v1/metadata" || route.Protocol != "inventory-v1" {
					t.Fatal("inventory routing evidence bypasses normal Agent metadata delivery")
				}
			}
		}
		for _, wire := range wires {
			if sample.Inventory != nil && wire.Path != "/api/v1/metadata" {
				t.Fatal("inventory wire evidence bypasses normal Agent metadata delivery")
			}
			var body []byte
			if sample.Processes != nil || sample.Connections != nil {
				message, err := model.DecodeMessage(wire.Body)
				if err != nil {
					t.Fatal("cannot decode recorded process group")
				}
				body, err = json.Marshal(message.Body)
				if err != nil {
					t.Fatal("cannot inspect recorded process group")
				}
				requestID, err := strconv.ParseUint(wire.Headers.Get(headers.RequestIDHeader), 10, 64)
				if err != nil || int(requestID&((1<<14)-1)) != ref.ChunkIndex {
					t.Fatal("wire chunk index differs from typed cycle")
				}
				timestamp, err := strconv.ParseInt(wire.Headers.Get(headers.TimestampHeader), 10, 64)
				if err != nil || timestamp != int64(ref.Offset/time.Second) {
					t.Fatal("wire collection time differs from typed cycle")
				}
			} else {
				body = auditDecompress(t, wire)
				check(body)
				if sample.Software != nil {
					var batch []json.RawMessage
					if json.Unmarshal(body, &batch) != nil || len(batch) != 1 {
						t.Fatal("software wire is not one complete snapshot")
					}
					body = batch[0]
				}
			}
			check(body)
			var actual, expected any
			if json.Unmarshal(body, &actual) != nil || json.Unmarshal(typed, &expected) != nil || !reflect.DeepEqual(actual, expected) {
				t.Fatal("decoded wire body differs from its sanitized typed sample")
			}
		}
	}
}

// Compare complete protocol-specific multisets, tolerating serializer splitting
// and wire request reordering. Source enums and fractional timestamps remain
// exact only in typed samples; wire timestamps are truncated to integer seconds.
func auditMetricEvidence(t *testing.T, series []*metrics.Serie, ref bundle.SampleRef, wires []bundle.WireReference, check func([]byte)) {
	t.Helper()
	union := map[uint64]bool{}
	memberships := map[string]map[uint64]bool{}
	for _, route := range ref.Routes {
		if memberships[route.Endpoint] == nil {
			memberships[route.Endpoint] = map[uint64]bool{}
		}
		for _, ordinal := range route.Ordinals {
			union[ordinal], memberships[route.Endpoint][ordinal] = true, true
		}
	}
	ordinals := make([]uint64, 0, len(union))
	for ordinal := range union {
		ordinals = append(ordinals, ordinal)
	}
	slices.Sort(ordinals)
	if len(ordinals) != len(series) {
		t.Fatal("metric route membership differs from typed series")
	}
	canonical := func(metric artifactWireMetric) string {
		metric.Tags = slices.Clone(metric.Tags)
		slices.Sort(metric.Tags)
		if len(metric.Tags) == 0 {
			metric.Tags = nil
		}
		data, err := json.Marshal(metric)
		if err != nil {
			t.Fatal("invalid decoded metric semantics")
		}
		return string(data)
	}
	expected := map[string]map[string]int{}
	for endpoint, members := range memberships {
		expected[endpoint] = map[string]int{}
		for i, serie := range series {
			if !members[ordinals[i]] {
				continue
			}
			value := artifactWireMetric{Name: serie.Name, Host: serie.Host, Device: serie.Device, Type: serie.MType.String(), Interval: serie.Interval}
			// Every metric protocol moves device tags into the device field;
			// the last such tag wins. Project that wire representation without
			// mutating the exact pre-serialization tags in the typed sample.
			serie.Tags.ForEach(func(tag string) {
				if device, ok := strings.CutPrefix(tag, "device:"); ok {
					value.Device = device
				} else {
					value.Tags = append(value.Tags, tag)
				}
			})
			for _, point := range serie.Points {
				value.Points = append(value.Points, [2]float64{float64(int64(point.Ts)), point.Value})
			}
			expected[endpoint][canonical(value)]++
		}
	}
	for _, wire := range wires {
		check(auditDecompress(t, wire))
		for _, value := range auditDecodeMetricWire(t, wire) {
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal("cannot inspect decoded metric semantics")
			}
			check(data)
			key := canonical(value)
			if expected[wire.Path][key] == 0 {
				t.Fatal("decoded metric series differs from its typed sample or protocol membership")
			}
			expected[wire.Path][key]--
		}
	}
	for _, members := range expected {
		for _, remaining := range members {
			if remaining != 0 {
				t.Fatal("typed metric series lacks matching wire evidence")
			}
		}
	}
}

func auditDecompress(t *testing.T, wire bundle.WireReference) []byte {
	t.Helper()
	encoding := wire.Headers.Get("Content-Encoding")
	if encoding == "" || encoding == "identity" {
		return wire.Body
	}
	kind, ok := map[string]string{"deflate": "zlib", "gzip": "gzip", "zstd": "zstd"}[encoding]
	if !ok {
		t.Fatal("unknown recorded compression format")
	}
	compressor := logscompression.NewComponent().NewCompressor(kind, 1)
	if compressor.ContentEncoding() != encoding {
		t.Fatal("required capture decompressor is unavailable")
	}
	data, err := compressor.Decompress(wire.Body)
	if err != nil {
		t.Fatal("cannot decompress recorded Agent body")
	}
	return data
}

func TestArtifactEvidenceComparisonFixtures(t *testing.T) {
	if logscompression.NewComponent().NewCompressor("zlib", 1).ContentEncoding() != "deflate" {
		t.Skip("zlib codec required")
	}
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
			// Full decoded-body equality includes AgentConfiguration, so the
			// typed flags must also survive the fixture's real Agent encoding.
			auditBundleEvidence(t, fixture, func([]byte) {})
		})
	}
}

func TestArtifactMetricComparisonProtocols(t *testing.T) {
	if logscompression.NewComponent().NewCompressor("zlib", 1).ContentEncoding() != "deflate" {
		t.Skip("zlib codec required")
	}
	for _, protocol := range []string{"v1", "v2", "v3", "v3beta"} {
		t.Run(protocol, func(t *testing.T) {
			if strings.HasPrefix(protocol, "v3") && logscompression.NewComponent().NewCompressor("zstd", 1).ContentEncoding() != "zstd" {
				t.Skip("zstd codec required")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			recorder := output.NewRecorder()
			pipeline, err := output.New(ctx, nil, "synthetic-audit-credential", recorder, output.Options{MetricProtocol: protocol})
			if err != nil {
				t.Fatal("cannot construct isolated metric audit pipeline")
			}
			defer pipeline.Close()
			series := []*metrics.Serie{
				{Name: "system.cpu.user", Host: "capture-host", Device: "device-test", Tags: tagset.CompositeTagsFromSlice([]string{"core:cpu0", "infra_mode:end_user_device"}), MType: metrics.APIGaugeType, Source: metrics.MetricSourceCPU, Interval: 15, Points: []metrics.Point{{Ts: 1.75, Value: 1.25}}},
				{Name: "system.mem.total", Host: "capture-host", MType: metrics.APIGaugeType, Source: metrics.MetricSourceMemory, Interval: 15, Points: []metrics.Point{{Ts: -0.25, Value: 1024}}},
				{Name: "system.disk.free", Host: "capture-host", Tags: tagset.CompositeTagsFromSlice([]string{"device:disk-one", "infra_mode:end_user_device"}), MType: metrics.APIGaugeType, Interval: 15, Points: []metrics.Point{{Ts: 2.75, Value: 4096}}},
				{Name: "system.disk.used", Host: "capture-host", Device: "overridden-device", Tags: tagset.CompositeTagsFromSlice([]string{"device:disk-two"}), MType: metrics.APIGaugeType, Interval: 15, Points: []metrics.Point{{Ts: 2.75, Value: 2048}}},
				{Name: "system.disk.total", Host: "capture-host", Tags: tagset.CompositeTagsFromSlice([]string{"device:first-device", "device:last-device"}), MType: metrics.APIGaugeType, Interval: 15, Points: []metrics.Point{{Ts: 2.75, Value: 6144}}},
			}
			typed, err := telemetry.NewMetricSample(series)
			if err != nil {
				t.Fatal("cannot create typed metric evidence")
			}
			owned, err := typed.AgentSeries()
			if err != nil {
				t.Fatal("cannot create independent metric source")
			}
			if pipeline.Serializer.SendIterableSeries(capture.NewSeriesSource(owned)) != nil || pipeline.Wait(ctx) != nil {
				t.Fatal("metric wire regeneration failed")
			}
			wires := recorder.Drain()
			if len(wires) == 0 {
				t.Fatal("metric wire regeneration produced no evidence")
			}
			before, err := json.Marshal(series)
			if err != nil {
				t.Fatal("cannot snapshot typed metric evidence")
			}
			ref := bundle.SampleRef{Stream: schema.Metrics, Routes: []bundle.RoutingEvidence{{PayloadID: 1, Ordinals: []uint64{1, 4, 8, 10, 15}, Endpoint: wires[0].Path, Protocol: protocol, Destination: "primary"}}}
			auditMetricEvidence(t, series, ref, wires, func(data []byte) {
				if bytes.Contains(data, []byte("synthetic-audit-credential")) {
					t.Fatal("credential escaped isolated recording")
				}
			})
			after, err := json.Marshal(series)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("metric audit mutated typed evidence")
			}
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

type artifactWireMetric struct {
	Name     string       `json:"metric"`
	Host     string       `json:"host"`
	Device   string       `json:"device"`
	Tags     []string     `json:"tags"`
	Type     string       `json:"type"`
	Interval int64        `json:"interval"`
	Points   [][2]float64 `json:"points"`
}

// Decode the actual wire representations independently of the serializer. v2
// and v3 carry source-derived origin metadata, not the exact source enum;
// timestamps in all three formats are integer seconds.
func auditDecodeMetricWire(t *testing.T, wire bundle.WireReference) []artifactWireMetric {
	t.Helper()
	data := auditDecompress(t, wire)
	if wire.Path == "/api/v1/series" {
		var value struct {
			Series []artifactWireMetric `json:"series"`
		}
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal("cannot decode recorded metric JSON")
		}
		return value.Series
	}
	fields := auditProtoFields(t, data)
	if wire.Path == "/api/v2/series" {
		var result []artifactWireMetric
		for _, raw := range fields[1] {
			f := auditProtoFields(t, raw)
			m := artifactWireMetric{Name: string(auditFirst(f[2])), Type: map[uint64]string{1: "count", 2: "rate", 3: "gauge"}[auditFieldNumber(f[5])], Interval: int64(auditFieldNumber(f[8]))}
			for _, tag := range f[3] {
				m.Tags = append(m.Tags, string(tag))
			}
			for _, resource := range f[1] {
				r := auditProtoFields(t, resource)
				switch string(auditFirst(r[1])) {
				case "host":
					m.Host = string(auditFirst(r[2]))
				case "device":
					m.Device = string(auditFirst(r[2]))
				}
			}
			for _, raw := range f[4] {
				p := auditProtoFields(t, raw)
				m.Points = append(m.Points, [2]float64{float64(int64(auditFieldNumber(p[2]))), math.Float64frombits(binary.LittleEndian.Uint64(auditFirst(p[1])))})
			}
			result = append(result, m)
		}
		return result
	}
	columns := auditProtoFields(t, auditFirst(fields[3]))
	c := map[int][]byte{}
	for id, v := range columns {
		c[id] = auditFirst(v)
	}
	stringsAt := func(id int) []string {
		v := []string{""}
		data := c[id]
		for len(data) > 0 {
			n := auditTakeUvarint(t, &data)
			if n > uint64(len(data)) {
				t.Fatal("invalid v3 dictionary")
			}
			v = append(v, string(data[:n]))
			data = data[n:]
		}
		return v
	}
	names, tags, resources := stringsAt(1), stringsAt(2), stringsAt(4)
	tagsets := [][]string{nil}
	for len(c[3]) > 0 {
		n := auditTakeSint(t, &c, 3)
		var values []string
		var index int64
		for range n {
			index += auditTakeSint(t, &c, 3)
			if index < 0 {
				values = append(values, tagsets[-index]...)
			} else {
				values = append(values, tags[index])
			}
		}
		tagsets = append(tagsets, values)
	}
	resourceSets := []map[string]string{nil}
	for len(c[5]) > 0 {
		n := auditTakeColumnUint(t, c, 5)
		set := map[string]string{}
		var kind, name int64
		for range n {
			kind += auditTakeSint(t, &c, 6)
			name += auditTakeSint(t, &c, 7)
			set[resources[kind]] = resources[name]
		}
		resourceSets = append(resourceSets, set)
	}
	var result []artifactWireMetric
	var nameID, tagID, resourceID, timestamp int64
	for len(c[10]) > 0 {
		typ := auditTakeColumnUint(t, c, 10)
		nameID += auditTakeSint(t, &c, 11)
		tagID += auditTakeSint(t, &c, 12)
		resourceID += auditTakeSint(t, &c, 13)
		m := artifactWireMetric{Name: names[nameID], Tags: slices.Clone(tagsets[tagID]), Host: resourceSets[resourceID]["host"], Device: resourceSets[resourceID]["device"], Type: map[uint64]string{1: "count", 2: "rate", 3: "gauge"}[typ&15], Interval: int64(auditTakeColumnUint(t, c, 14))}
		for n := auditTakeColumnUint(t, c, 15); n > 0; n-- {
			timestamp += auditTakeSint(t, &c, 16)
			var value float64
			switch typ & 0xf0 {
			case 0x10:
				value = float64(auditTakeSint(t, &c, 17))
			case 0x20:
				if len(c[18]) < 4 {
					t.Fatal("short float32")
				}
				value = float64(math.Float32frombits(binary.LittleEndian.Uint32(c[18])))
				c[18] = c[18][4:]
			case 0x30:
				if len(c[19]) < 8 {
					t.Fatal("short float64")
				}
				value = math.Float64frombits(binary.LittleEndian.Uint64(c[19]))
				c[19] = c[19][8:]
			}
			m.Points = append(m.Points, [2]float64{float64(timestamp), value})
		}
		result = append(result, m)
	}
	return result
}

func auditFirst(values [][]byte) []byte {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}
func auditFieldNumber(values [][]byte) uint64 {
	value, _ := binary.Uvarint(auditFirst(values))
	return value
}
func auditTakeUvarint(t *testing.T, data *[]byte) uint64 {
	t.Helper()
	value, n := binary.Uvarint(*data)
	if n <= 0 {
		t.Fatal("invalid wire varint")
	}
	*data = (*data)[n:]
	return value
}
func auditTakeColumnUint(t *testing.T, c map[int][]byte, id int) uint64 {
	data := c[id]
	value := auditTakeUvarint(t, &data)
	c[id] = data
	return value
}
func auditTakeSint(t *testing.T, c *map[int][]byte, id int) int64 {
	v := auditTakeColumnUint(t, *c, id)
	return int64(v>>1) ^ -int64(v&1)
}
func auditProtoFields(t *testing.T, data []byte) map[int][][]byte {
	t.Helper()
	result := map[int][][]byte{}
	for len(data) > 0 {
		key := auditTakeUvarint(t, &data)
		id, kind := int(key>>3), key&7
		var value []byte
		switch kind {
		case 0:
			before := data
			auditTakeUvarint(t, &data)
			value = before[:len(before)-len(data)]
		case 1:
			if len(data) < 8 {
				t.Fatal("short protobuf fixed64")
			}
			value, data = data[:8], data[8:]
		case 2:
			n := auditTakeUvarint(t, &data)
			if n > uint64(len(data)) {
				t.Fatal("short protobuf bytes")
			}
			value, data = data[:n], data[n:]
		default:
			t.Fatal("unsupported protobuf field")
		}
		result[id] = append(result[id], value)
	}
	return result
}
