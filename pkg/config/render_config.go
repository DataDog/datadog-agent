// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build ignore

package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

// context contains the context used to render the config file template
type context struct {
	Common            bool
	Agent             bool
	Python            bool // Sub-option of Agent
	BothPythonPresent bool // Sub-option of Agent - Python
	Metadata          bool
	Profiling         bool
	Dogstatsd         bool
	LogsAgent         bool
	JMX               bool
	Autoconfig        bool
	Logging           bool
	Autodiscovery     bool
	DockerTagging     bool
	Kubelet           bool
	KubernetesTagging bool
	ECS               bool
	Containerd        bool
	CRI               bool
	ProcessAgent      bool
	SystemProbe       bool
	KubeApiServer     bool
	TraceAgent        bool
	ClusterChecks     bool
	CloudFoundryBBS   bool
	Compliance        bool
	SNMP              bool
	SecurityModule    bool
	NetworkModule     bool // Sub-module of System Probe
}

func mkContext(buildType string) context {
	buildType = strings.ToLower(buildType)

	agentContext := context{
		Common:            true,
		Agent:             true,
		Python:            true,
		Metadata:          true,
		Profiling:         false, // NOTE: hidden for now
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
		SNMP:              true,
		SecurityModule:    true,
	}

	switch buildType {
	case "agent-py3":
		return agentContext
	case "agent-py2py3":
		agentContext.BothPythonPresent = true
		return agentContext
	case "iot-agent":
		return context{
			Common:    true,
			Agent:     true,
			Metadata:  true,
			Dogstatsd: true,
			LogsAgent: true,
			Logging:   true,
		}
	case "system-probe":
		return context{
			SystemProbe:   true,
			NetworkModule: true,
		}
	case "dogstatsd":
		return context{
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
			Common:        true,
			Logging:       true,
			KubeApiServer: true,
			ClusterChecks: true,
		}
	case "dcacf":
		return context{
			Common:          true,
			Logging:         true,
			ClusterChecks:   true,
			CloudFoundryBBS: true,
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

	fmt.Println("Successfully wrote", destFile)
}
