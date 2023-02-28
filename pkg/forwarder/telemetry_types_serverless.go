// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless
// +build serverless

package forwarder

// k8SResource is a node type for k8s.
type k8SResource int32

// String returns the string value of this k8SResource.
func (k8SResource) String() string { return "" }

// nodeTypes returns the current existing NodesTypes as a slice to iterate over.
func nodeTypes() []k8SResource { return nil }
