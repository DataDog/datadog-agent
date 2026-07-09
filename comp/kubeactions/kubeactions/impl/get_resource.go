// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeactionsimpl

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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const (
	// defaultExecutorTimeout is the default timeout applied to API calls made by
	// the get_resource executor.
	defaultExecutorTimeout = 10 * time.Second

	// maxResourceOutputSize is the maximum size of the resource output in bytes.
	// It is set to 1.5MB to avoid filling the database with large resources;
	// the largest expected resource is ~1MB for a configMap, so 1.5MB is a safe
	// margin.
	maxResourceOutputSize int = 1.5 * 1024 * 1024 // 1.5MB
)

var protectedResourceKinds = []string{"secret", "secrets"}

// ErrUnsupportedFormat is returned when the requested output format is not supported.
var ErrUnsupportedFormat = errors.New("unsupported output format")

// GetResourceExecutor executes get_resource actions.
type GetResourceExecutor struct {
	dynamicClient dynamic.Interface
	scrubber      *scrubber.Scrubber
}

// NewGetResourceExecutor creates a new GetResourceExecutor.
func NewGetResourceExecutor(dynamicClient dynamic.Interface) *GetResourceExecutor {
	scrb := scrubber.NewWithDefaults()

	// If the user has set a custom list of sensitive words, add a replacer for
	// them. This is originally added for the orchestrator explorer to scrub
	// sensitive words from the resource output; see
	// pkg/orchestrator/config/config.go.OrchestratorConfig.Load() for details.
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

// Execute retrieves the specified Kubernetes resource and returns it as a JSON
// payload keyed by the resource name.
func (e *GetResourceExecutor) Execute(ctx context.Context, in kubeactions.GetResourceInputs) kubeactions.ExecutionResult {
	namespace := strings.ToLower(in.Namespace)
	name := strings.ToLower(in.Name)
	apiVersion := strings.ToLower(in.APIVersion)
	kind := strings.ToLower(in.Kind)

	if apiVersion == "" {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: "apiVersion is required to get resource",
		}
	}

	// Prevent the executor from being used to get protected resources for
	// security reasons, even if the user has permissions to do so.
	if slices.Contains(protectedResourceKinds, strings.ToLower(kind)) {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("getting %s is not allowed for security reasons", kind),
		}
	}

	// Parse apiVersion into group and version for the dynamic client.
	// Core resources have no group (e.g. "v1"), others use "group/version"
	// (e.g. "apps/v1").
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
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to get resource %s: %v", resourceName, err),
		}
	}

	data, err := obj.MarshalJSON()
	if err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to marshal resource %s to JSON: %v", resourceName, err),
		}
	}

	// Scrub the output to prevent security leaks.
	output, err := e.scrubber.ScrubJSON(data)
	if err != nil {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("failed to scrub resource %s: %v", resourceName, err),
		}
	}

	// Try to compact the JSON data.
	buff := &bytes.Buffer{}
	if json.Compact(buff, output) == nil {
		output = buff.Bytes()
	}

	output = bytes.TrimSpace(output)
	if len(output) > maxResourceOutputSize {
		return kubeactions.ExecutionResult{
			Status:  kubeactions.StatusFailed,
			Message: fmt.Sprintf("resource is too large (%d bytes), maximum size is %d bytes", len(output), maxResourceOutputSize),
		}
	}

	return kubeactions.ExecutionResult{
		Status:  kubeactions.StatusSuccess,
		Message: fmt.Sprintf("get resource %s success", resourceName),
		Payloads: map[string][]byte{
			resourceName: output,
		},
	}
}
