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
	"regexp"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
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
	scrubber      *scrubber.Scrubber
}

// Ensure interface compliance at compile time
var _ Executor = (*GetResourceExecutor)(nil)

var (
	// ErrUnsupportedFormat is returned when the requested output format is not supported
	ErrUnsupportedFormat = errors.New("unsupported output format")
)

// NewGetResourceExecutor creates a new GetResourceExecutor
func NewGetResourceExecutor(dynamicClient dynamic.Interface) *GetResourceExecutor {
	scrb := scrubber.NewWithDefaults()

	// if the user has set a custom list of sensitive words, add a replacer for them
	// this is originally added for the orchestrator explorer to scrub sensitive words from the resource output
	// see pkg/orchestrator/config/config.go.OrchestratorConfig.Load() for more details.
	if pkgconfigsetup.Datadog().IsConfigured("orchestrator_explorer.custom_sensitive_words") {
		sensitiveWords := pkgconfigsetup.Datadog().GetStringSlice("orchestrator_explorer.custom_sensitive_words")
		regex := regexp.MustCompile(strings.Join(sensitiveWords, "|"))
		scrb.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
			Regex: regex,
			Repl:  []byte("********"),
		})
	}

	return &GetResourceExecutor{
		dynamicClient: dynamicClient,
		scrubber:      scrb,
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

	// scrub the output to prevent security leaks
	output, err := e.scrubber.ScrubJSON(data)
	if err != nil {
		return ExecutionResult{
			Status:  StatusFailed,
			Message: fmt.Sprintf("failed to scrub resource %s: %v", resourceName, err),
		}
	}

	// try to compact the JSON data
	buff := &bytes.Buffer{}
	if json.Compact(buff, output) == nil {
		// we successfully compacted the JSON data
		output = buff.Bytes()
	}

	output = bytes.TrimSpace(output)
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
