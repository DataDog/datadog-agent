package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/avast/retry-go/v4"
)

type endpointOutput = workloadmeta.PodContainerMetadataResponse

// const endpointPath = "/alpha/instrumentation/pod-container-metadata"

func main() {
	endpoint := os.Args[1]
	request := os.Args[2]

	basePath, err := baseURL(
		os.Getenv("DD_AGENT_HOST"),
		os.Getenv("DD_AGENT_PORT"),
		endpoint,
	)
	if err != nil {
		panic(err)
	}

	queryParams, err := urlParams(os.Getenv("POD_NAME"), os.Getenv("POD_NS"))
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := util.GetClient(false)
	url := basePath + "?" + queryParams + "&request=" + request

	data, err := retry.DoWithData(
		func() (endpointOutput, error) {
			var data endpointOutput
			body, err := util.DoGet(client, url, util.LeaveConnectionOpen)
			if err != nil {
				return data, err
			}

			if err := json.Unmarshal(body, &data); err != nil {
				return data, err
			}

			return data, nil
		},
		retry.Context(ctx),
		retry.Attempts(0),
		retry.WrapContextErrorWithLastError(true),
	)
	if err != nil {
		panic(err)
	}

	rootPath := "/dd-entry-data"
	for name, containerData := range data.Containers {
		if err := writeContainerData(rootPath, name, containerData); err != nil {
			panic(err)
		}
	}
}

func writeContainerData(root string, name string, c workloadmeta.PodContainerMetadata) error {
	encoded, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("error encoding data for container %s: %w", name, err)
	}

	containerDir := filepath.Join(root, name)
	if err := os.Mkdir(containerDir, 0750); err != nil {
		return fmt.Errorf("could not create directory for %s: %w", name, err)
	}

	dataPath := filepath.Join(containerDir, "data.json")
	f, err := os.Create(dataPath)
	if err != nil {
		return fmt.Errorf("could not create data file for %s: %w", name, err)
	}

	defer f.Close()

	_, err = f.Write(encoded)
	if err != nil {
		return fmt.Errorf("failed writing data for %s: %w", name, err)
	}

	// copy runBinary
	runPath := filepath.Join(containerDir, "run")
	cmd := exec.Command("cp", "/dd-source/entry", runPath)
	return cmd.Run()
}

func baseURL(host, port, endpoint string) (string, error) {
	if host == "" {
		return "", errors.New("missing hostname")
	}

	if port == "" {
		return "", errors.New("missing port")
	}

	if endpoint == "" {
		return "", errors.New("missing endpoint")
	}

	return fmt.Sprintf("http://%s:%s", host, port) + endpoint, nil
}

func urlParams(name, ns string) (string, error) {
	if name == "" {
		return "", errors.New("empty pod name")
	}
	if ns == "" {
		return "", errors.New("empty namespace")
	}

	return fmt.Sprintf("name=%s&ns=%s", name, ns), nil
}
