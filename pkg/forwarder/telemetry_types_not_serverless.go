// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package forwarder

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

// k8SResource is a node type for k8s.
type k8SResource model.K8SResource

// String returns the string value of this k8SResource.
func (r k8SResource) String() string {
	return model.K8SResource(r).String()
}

// nodeTypes returns the current existing NodesTypes as a slice to iterate over.
func nodeTypes() []k8SResource {
	types := orchestrator.NodeTypes()
	resources := make([]k8SResource, len(types))
	for _, v := range types {
		resources = append(resources, k8SResource(v))
	}
	return resources
}
