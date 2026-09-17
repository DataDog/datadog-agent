// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"sort"
	"strings"
	"text/template"

	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
)

//go:embed fixtures/datasecurity.yaml
var datasecurityYAMLTmpl string

//go:embed fixtures/scenarios/*.yaml
var scenarioFS embed.FS

type postgresScanScenario struct {
	name     string
	scanData []map[string]any
	expected []*sds.SdsResultPayload
}

type scenarioFile struct {
	Name     string           `yaml:"name"`
	ScanData []map[string]any `yaml:"scan_data"`
	Expected []map[string]any `yaml:"expected"`
}

var postgresScanScenarios = loadPostgresScanScenarios()

func datasecurityCheckYAML() string {
	scanData := make([]map[string]any, 0)
	for _, scenario := range postgresScanScenarios {
		scanData = append(scanData, scenario.scanData...)
	}
	body, err := yaml.Marshal(scanData)
	if err != nil {
		panic("datasecurity scan_data: " + err.Error())
	}
	return mustRender("datasecurity.yaml", datasecurityYAMLTmpl, struct{ ScanData string }{
		ScanData: indentLines(string(body), 6),
	})
}

func loadPostgresScanScenarios() []postgresScanScenario {
	files := mustReadScenarioFiles()
	scenarios := make([]postgresScanScenario, 0, len(files))
	for _, file := range files {
		expected := make([]*sds.SdsResultPayload, 0, len(file.Expected))
		for _, raw := range file.Expected {
			expected = append(expected, mustSDSResult(file.Name, raw))
		}
		scenarios = append(scenarios, postgresScanScenario{
			name:     file.Name,
			scanData: file.ScanData,
			expected: expected,
		})
	}
	return scenarios
}

func mustReadScenarioFiles() []scenarioFile {
	entries, err := fs.ReadDir(scenarioFS, "fixtures/scenarios")
	if err != nil {
		panic("fixtures/scenarios: " + err.Error())
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	files := make([]scenarioFile, 0, len(names))
	for _, name := range names {
		path := "fixtures/scenarios/" + name
		raw, err := scenarioFS.ReadFile(path)
		if err != nil {
			panic(path + ": " + err.Error())
		}
		var file scenarioFile
		if err := yaml.Unmarshal(raw, &file); err != nil {
			panic(path + ": " + err.Error())
		}
		if file.Name == "" {
			file.Name = strings.TrimSuffix(name, ".yaml")
		}
		files = append(files, file)
	}
	return files
}

func mustSDSResult(scenario string, raw map[string]any) *sds.SdsResultPayload {
	encoded, err := json.Marshal(raw)
	if err != nil {
		panic(scenario + ": " + err.Error())
	}
	var payload sds.SdsResultPayload
	if err := protojson.Unmarshal(encoded, &payload); err != nil {
		panic(scenario + ": " + err.Error())
	}
	return &payload
}

func mustRender(name, src string, data any) string {
	tmpl := template.Must(template.New(name).Parse(src))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(name + ": " + err.Error())
	}
	return buf.String()
}

func indentLines(s string, n int) string {
	prefix := strings.Repeat(" ", n)
	s = strings.TrimRight(s, "\n")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
