// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package executors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
)

const (
	// maxResourceOutputSize is the maximum size of the resource output in bytes
	// it is set to 1.5MB to avoid filling the database with large resources
	// largest expected resource is a 1MB for configMap, so 1.5MB is a safe margin
	maxResourceOutputSize int = 1.5 * 1024 * 1024 // 1.5MB
)

var protectedResourceKinds = []string{"secret", "secrets"}

type GetResourceExecutor struct {
	dynamicClient dynamic.Interface
}

// Ensure interface compliance at compile time
var _ Executor = (*GetResourceExecutor)(nil)

var (
	// ErrUnsupportedFormat is returned when the requested output format is not supported
	ErrUnsupportedFormat = errors.New("unsupported output format")
)

// NewGetResourceExecutor creates a new GetResourceExecutor
func NewGetResourceExecutor(dynamicClient dynamic.Interface) *GetResourceExecutor {
	return &GetResourceExecutor{
		dynamicClient: dynamicClient,
	}
}

func getResourceName(kind, namespace, name string) string {
	return path.Join(kind, namespace, name)
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

	// prevent the executor from being used to get protected resources for security reasons, even if the user has permissions to do so, we don't want to allow that
	if slices.Contains(protectedResourceKinds, strings.ToLower(kind)) {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("getting %s is not allowed for security reasons", kind),
		}
	}

	// parse apiVersion into group and version for the dynamic client.
	// core resources have no group (e.g. "v1"), others use "group/version" (e.g. "apps/v1").
	var group, version string
	if parts := strings.SplitN(apiVersion, "/", 2); len(parts) == 2 {
		group, version = parts[0], parts[1]
	} else {
		version = apiVersion
	}

	resourceName := getResourceName(kind, namespace, name)

	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: kind}

	ctx, cancel := context.WithTimeout(ctx, defaultExecutorTimeout)
	defer cancel()

	obj, err := e.dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to get resource %s: %v", resourceName, err),
		}
	}

	data, err := obj.MarshalJSON()
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to marshal resource %s to JSON: %v", resourceName, err),
		}
	}

	buff := bytes.Buffer{}

	// try to compact the JSON data
	if json.Compact(&buff, data) != nil {
		// if the JSON data is not compactable, we will use the original data
		buff.Reset()
		buff.Write(data)
	}

	output := bytes.TrimSpace(buff.Bytes())
	if len(output) > maxResourceOutputSize {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("resource is too large (%d bytes), maximum size is %d bytes", len(output), maxResourceOutputSize),
		}
	}

	return ExecutionResult{
		Status:  StatusSuccess,
		Message: fmt.Sprintf("get resource %s success", resourceName),
		Payloads: map[string][]byte{
			resourceName: output,
		},
	}
}
