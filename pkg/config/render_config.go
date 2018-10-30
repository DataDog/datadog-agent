// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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
	Metadata          bool
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
	CRI               bool
	ProcessAgent      bool
	NetworkTracer     bool
	KubeApiServer     bool
	TraceAgent        bool
}

func mkContext(buildType string) context {
	buildType = strings.ToLower(buildType)

	switch buildType {
	case "agent":
		return context{
			Common:            true,
			Agent:             true,
			Metadata:          true,
			Dogstatsd:         true,
			LogsAgent:         true,
			JMX:               true,
			Autoconfig:        true,
			Logging:           true,
			Autodiscovery:     true,
			DockerTagging:     true,
			KubernetesTagging: true,
			ECS:               true,
			CRI:               true,
			ProcessAgent:      true,
			TraceAgent:        true,
			Kubelet:           true,
			KubeApiServer:     true, // TODO: remove when phasing out from node-agent
		}
	case "network-tracer":
		return context{
			NetworkTracer: true,
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
