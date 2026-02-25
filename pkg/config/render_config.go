// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

package main

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"gopkg.in/yaml.v3"
)

// context contains the context used to render the config file template
type context struct {
	OS                  string
	Common              bool
	Agent               bool
	CoreAgent           bool
	Dogstatsd           bool
	LogsAgent           bool
	Logging             bool
	DockerTagging       bool
	Kubelet             bool
	KubernetesTagging   bool
	ECS                 bool
	Containerd          bool
	KubeApiServer       bool
	TraceAgent          bool
	ClusterAgent        bool
	ClusterChecks       bool
	AdmissionController bool
	CloudFoundry        bool
	PrivateActionRunner bool
}

func mkContext(buildType string, osName string) context {
	buildType = strings.ToLower(buildType)

	switch buildType {
	case "agent-py3":
		return context{
			OS:                  osName,
			Common:              true,
			Agent:               true,
			CoreAgent:           true,
			Dogstatsd:           true,
			LogsAgent:           true,
			Logging:             true,
			DockerTagging:       true,
			KubernetesTagging:   true,
			ECS:                 true,
			TraceAgent:          true,
			Kubelet:             true,
			KubeApiServer:       true,                // TODO: remove when phasing out from node-agent
			PrivateActionRunner: true,
		}
	case "iot-agent":
		return context{
			OS:        osName,
			Common:    true,
			Agent:     true,
			Dogstatsd: true,
			LogsAgent: true,
			Logging:   true,
		}
	case "dogstatsd":
		return context{
			OS:                osName,
			Common:            true,
			Dogstatsd:         true,
			DockerTagging:     true,
			Logging:           true,
			KubernetesTagging: true,
			ECS:               true,
			TraceAgent:        true,
			Kubelet:           true,
		}
	case "dca":
		return context{
			OS:                  osName,
			ClusterAgent:        true,
			Common:              true,
			Logging:             true,
			KubeApiServer:       true,
			ClusterChecks:       true,
			AdmissionController: true,
		}
	case "dcacf":
		return context{
			OS:            osName,
			ClusterAgent:  true,
			Common:        true,
			Logging:       true,
			ClusterChecks: true,
			CloudFoundry:  true,
		}
	// security-agent and system-probe use their own templating file, they only require OS
	case "security-agent":
		return context{
			OS: osName,
		}
	case "system-probe":
		return context{
			OS: osName,
		}
	}

	return context{}
}

func render(destFile string, tplFile string, component string, osName string) {
	f, err := os.Create(destFile)
	if err != nil {
		panic(err)
	}

	tplFilename := filepath.Base(tplFile)

	t := template.Must(template.New(tplFilename).ParseFiles(tplFile))
	err = t.Execute(f, mkContext(component, osName))
	if err != nil {
		panic(err)
	}

	if err := f.Close(); err != nil {
		panic(err)
	}
}

func renderAll(destFolder string, tplFolder string) {
	for component, templateName := range map[string]string{
		"agent-py3":      "config_template.yaml",
		"iot-agent":      "config_template.yaml",
		"dogstatsd":      "config_template.yaml",
		"dca":            "config_template.yaml",
		"dcacf":          "config_template.yaml",
		"system-probe":   "system-probe_template.yaml",
		"security-agent": "security-agent_template.yaml",
	} {
		for _, osName := range []string{"windows", "darwin", "linux"} {
			destFile := filepath.Join(destFolder, component+"_"+osName+".yaml")
			render(destFile, filepath.Join(tplFolder, templateName), component, osName)
			fmt.Println("Successfully wrote", destFile)
		}
	}
}

func main() {
	if len(os.Args) == 3 {
		renderAll(os.Args[1], os.Args[2])
		return
	}
	if len(os.Args) != 4 {
		panic("please use 'go run render_config.go <component_name> <template_file> <destination_file>'\nOr `go run render_config.go <dest_folder> <template_foler>` to generate all possible templates")
	}

	component := os.Args[1]
	tplFile, _ := filepath.Abs(os.Args[2])
	destFile, _ := filepath.Abs(os.Args[3])

	render(destFile, tplFile, component, runtime.GOOS)
	if err := lint(destFile); err != nil {
		panic(err)
	}

	fmt.Println("Successfully wrote", destFile)
}

// lint reads the YAML file at destFile, unmarshals it into a yaml.Node,
// re-encodes it, and compares the output to detect unintended changes.
// It returns an error if the re-encoded content differs from the original.
//
// The intent is to ensure that there are no large or confusing changes
// when the config is processed by yaml.Node. Fleet automation makes
// configuration changes and we want to ensure the input and output
// are reasonably stable.
func lint(destFile string) error {
	originalBytes, err := os.ReadFile(destFile)
	if err != nil {
		return fmt.Errorf("lint: failed to read file: %w", err)
	}

	// Normalize CRLF to LF to avoid platform-specific differences.
	normalized := bytes.ReplaceAll(originalBytes, []byte("\r"), []byte(""))

	var root yaml.Node
	if err := yaml.Unmarshal(normalized, &root); err != nil {
		return fmt.Errorf("lint: YAML unmarshal failed: %w", err)
	}
	if len(root.Content) == 0 {
		// Add a single lint_testing node to the original bytes.
		//
		// if there are no nodes then all comments are removed, so this
		// allows us to make a comparison even for files which only have comments,
		// such as system-probe.yaml.
		normalized = append(normalized, []byte("lint_testing: true # ignore me\n")...)
		if err := yaml.Unmarshal(normalized, &root); err != nil {
			return fmt.Errorf("lint: YAML unmarshal failed: %w", err)
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return fmt.Errorf("lint: YAML re-encode failed: %w", err)
	}

	reencoded := buf.Bytes()

	if !bytes.Equal(normalized, reencoded) {
		origLines := difflib.SplitLines(string(normalized))
		reencLines := difflib.SplitLines(string(reencoded))
		ud := difflib.ContextDiff{
			A:        origLines,
			B:        reencLines,
			FromFile: "rendered (original)",
			ToFile:   "rendered (re-encoded)",
			Context:  3,
		}
		diff, _ := difflib.GetContextDiffString(ud)
		return fmt.Errorf("linting %s: re-encoding YAML changed the content; please verify template correctness\n\nDiff:\n%s", destFile, diff)
	}
	return nil
}
