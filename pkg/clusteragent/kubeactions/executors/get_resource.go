// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package executors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"k8s.io/client-go/kubernetes"

	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
)

const (
	maxResourceOutputSize = 4 * 1024 // 4KB
)

type GetResourceExecutor struct {
	clientset kubernetes.Interface
}

// Ensure interface compliance at compile time
var _ Executor = (*GetResourceExecutor)(nil)

var (
	// ErrUnsupportedFormat is returned when the requested output format is not supported
	ErrUnsupportedFormat = errors.New("unsupported output format")
)

// NewGetResourceExecutor creates a new GetResourceExecutor
func NewGetResourceExecutor(clientset kubernetes.Interface) *GetResourceExecutor {
	return &GetResourceExecutor{
		clientset: clientset,
	}
}

// Execute retrieves the specified Kubernetes resource and returns it as JSON string in the message field of ExecutionResult
func (e *GetResourceExecutor) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	resource := action.Resource
	namespace := strings.ToLower(resource.GetNamespace())
	name := strings.ToLower(resource.GetName())
	apiVersion := strings.ToLower(resource.GetApiVersion())
	kind := strings.ToLower(resource.GetKind())

	if apiVersion == "" {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: "apiVersion is required to get resource",
		}
	}

	// prevent the executor from being used to get secrets for security reasons, even if the user has permissions to do so, we don't want to allow that
	if strings.Contains(kind, "secret") {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: "getting secrets is not allowed for security reasons",
		}
	}

	log.Infof("Getting resource %s/%s of type %s", namespace, name, resource.Kind)

	// build the raw REST request to get the resource as unstructured JSON
	var path string

	// the api version for core resources does not contain a '/'.
	// and the path for core resources is /api/....
	// as for all other resources the path is /apis/...
	var apiPrefix string
	if !strings.Contains(apiVersion, "/") {
		apiPrefix = "/api"
	} else {
		apiPrefix = "/apis"
	}

	// resource.GetApiVersion() returns group/version, it will automagically handle adding the group prefix if needed
	// or not adding it for core resources
	if namespace == "" {
		path, _ = url.JoinPath(apiPrefix, apiVersion, kind, name)
	} else {
		path, _ = url.JoinPath(apiPrefix, apiVersion, "namespaces", namespace, kind, name)
	}

	ctx, cancel := context.WithTimeout(ctx, defaultExecutorTimeout)
	defer cancel()

	log.Debugf("get_resource using path '%s'", path)
	data, err := e.clientset.CoreV1().RESTClient().Get().AbsPath(path).Do(ctx).Raw()
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to get resource: %v -- raw response body: %s", err, string(data)),
		}
	}

	outputFormat := "json"
	if output := action.GetGetResource_().GetOutputFormat(); output != "" {
		outputFormat = strings.ToLower(output)
	}

	output, err := formatOutput(data, outputFormat)
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to format output to %s: %v", outputFormat, err),
		}
	}

	output = bytes.TrimSpace(output)
	if len(output) > maxResourceOutputSize {
		log.Warnf("output for resource %s/%s of type %s is too large (%d bytes), truncating to %d bytes", namespace, name, kind, len(output), maxResourceOutputSize)
		output = output[:maxResourceOutputSize]
	}

	return ExecutionResult{
		Status:  StatusSuccess,
		Message: fmt.Sprintf("get resource %s/%s success", kind, name),
		Payloads: map[string][]byte{
			"resource": []byte(output),
		},
	}
}

func formatOutput(data []byte, format string) ([]byte, error) {
	switch format {
	case "json":
		return data, nil
	case "yaml":
		jsonData := data
		yamlData, err := yaml.JSONToYAML(jsonData)
		if err != nil {
			return nil, fmt.Errorf("failed to convert resource JSON to YAML: %v", err)
		}
		return yamlData, nil
	default:
		return nil, ErrUnsupportedFormat
	}
}
