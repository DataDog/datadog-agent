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
	OS                               string
	Common                           bool
	Agent                            bool
	Python                           bool // Sub-option of Agent
	Metadata                         bool
	InternalProfiling                bool
	Dogstatsd                        bool
	LogsAgent                        bool
	JMX                              bool
	Autoconfig                       bool
	Logging                          bool
	Autodiscovery                    bool
	DockerTagging                    bool
	Kubelet                          bool
	KubernetesTagging                bool
	ECS                              bool
	Containerd                       bool
	CRI                              bool
	ProcessAgent                     bool
	SystemProbe                      bool
	KubeApiServer                    bool
	TraceAgent                       bool
	ClusterAgent                     bool
	ClusterChecks                    bool
	AdmissionController              bool
	CloudFoundryBBS                  bool
	CloudFoundryCC                   bool
	Compliance                       bool
	SNMP                             bool
	SecurityModule                   bool
	SecurityAgent                    bool
	SBOM                             bool // enables CSM Vulnerability Management
	NetworkModule                    bool // Sub-module of System Probe
	UniversalServiceMonitoringModule bool // Sub-module of System Probe
	DataStreamsModule                bool // Sub-module of System Probe
	PingModule                       bool // Sub-module of System Probe
	TracerouteModule                 bool // Sub-module of System Probe
	PrometheusScrape                 bool
	OTLP                             bool
	APMInjection                     bool
	NetworkPath                      bool
	ApplicationMonitoring            bool
}

func mkContext(buildType string) context {
	buildType = strings.ToLower(buildType)

	agentContext := context{
		OS:                runtime.GOOS,
		Common:            true,
		Agent:             true,
		Python:            true,
		Metadata:          true,
		InternalProfiling: false, // NOTE: hidden for now
		Dogstatsd:         true,
		LogsAgent:         true,
		JMX:               true,
		Autoconfig:        true,
		Logging:           true,
		Autodiscovery:     true,
		DockerTagging:     true,
		KubernetesTagging: true,
		ECS:               true,
		Containerd:        true,
		CRI:               true,
		ProcessAgent:      true,
		TraceAgent:        true,
		Kubelet:           true,
		KubeApiServer:     true, // TODO: remove when phasing out from node-agent
		Compliance:        true,
		SBOM:              true,
		SNMP:              true,
		PrometheusScrape:  true,
		OTLP:              true,
		NetworkPath:       true,
	}

	switch buildType {
	case "agent-py3":
		return agentContext
	case "iot-agent":
		return context{
			OS:        runtime.GOOS,
			Common:    true,
			Agent:     true,
			Metadata:  true,
			Dogstatsd: true,
			LogsAgent: true,
			Logging:   true,
		}
	case "system-probe":
		return context{
			OS:                               runtime.GOOS,
			SystemProbe:                      true,
			NetworkModule:                    true,
			UniversalServiceMonitoringModule: true,
			DataStreamsModule:                true,
			SecurityModule:                   true,
			PingModule:                       true,
			TracerouteModule:                 true,
		}
	case "dogstatsd":
		return context{
			OS:                runtime.GOOS,
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
			OS:                  runtime.GOOS,
			ClusterAgent:        true,
			Common:              true,
			Logging:             true,
			KubeApiServer:       true,
			ClusterChecks:       true,
			AdmissionController: true,
		}
	case "dcacf":
		return context{
			OS:              runtime.GOOS,
			ClusterAgent:    true,
			Common:          true,
			Logging:         true,
			ClusterChecks:   true,
			CloudFoundryBBS: true,
			CloudFoundryCC:  true,
		}
	case "security-agent":
		return context{
			OS:            runtime.GOOS,
			SecurityAgent: true,
		}
	case "apm-injection":
		return context{
			OS:           runtime.GOOS,
			APMInjection: true,
		}
	case "application-monitoring":
		return context{
			OS:                    runtime.GOOS,
			ApplicationMonitoring: true,
		}
	}

	return context{}
}

func main() {
	if len(os.Args[1:]) != 3 {
		panic("please use 'go run render_config.go <component_name> <template_file> <destination_file>'")
	}

	component := os.Args[1]
	tplFile, _ := filepath.Abs(os.Args[2])
	tplFilename := filepath.Base(tplFile)
	destFile, _ := filepath.Abs(os.Args[3])

	f, err := os.Create(destFile)
	if err != nil {
		panic(err)
	}

	t := template.Must(template.New(tplFilename).ParseFiles(tplFile))
	err = t.Execute(f, mkContext(component))
	if err != nil {
		panic(err)
	}

	if err := f.Close(); err != nil {
		panic(err)
	}

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
		return fmt.Errorf("lint: re-encoding YAML changed the content; please verify template correctness\n\nDiff:\n%s", diff)
	}
	return nil
}
