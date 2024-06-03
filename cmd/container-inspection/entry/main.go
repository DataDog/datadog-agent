package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"

	"github.com/DataDog/datadog-agent/comp/containerinspection"
)

func main() {
	ddDataPath := os.Args[1]
	argv0, argv, env, err := command(ddDataPath, os.Environ())
	if err != nil {
		log.Fatal(fmt.Errorf("error setting up entrypoint command: %w", err))
	}

	err = syscall.Exec(argv0, argv, env)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to exec %w", err))
	}
}

func command(fromPath string, env []string) (string, []string, []string, error) {
	if fromPath == "" {
		return "", nil, nil, fmt.Errorf("empty data path")
	}

	data, err := os.ReadFile(fromPath)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to read file %s: %w", fromPath, err)
	}

	var e containerinspection.ContainerMetadata
	err = json.Unmarshal(data, &e)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed decoding entrypoint data: %w", err)
	}

	argv, env, err := analyze(e.Cmd, env)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed analysiss: %w", err)
	}

	argv0, err := exec.LookPath(argv[0])
	if err != nil {
		return "", nil, nil, fmt.Errorf("no executable found for %s: %w", argv[0], err)
	}

	return argv0, argv, env, nil
}

func analyze(argv, env []string) ([]string, []string, error) {
	// This is where we can hook into things like:
	// https://github.com/DataDog/service_discovery_support/blob/main/apm/detect.go#L37
	return argv, env, nil
}
