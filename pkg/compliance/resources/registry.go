// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import "github.com/DataDog/datadog-agent/pkg/compliance"

var (
	resourceRegistry = map[compliance.ResourceKind]*ResourceHandler{}
)

type ResourceHandler struct {
	Resolver       Resolver
	ReportedFields []string
}

func RegisterHandler(name compliance.ResourceKind, resolver Resolver, fields []string) {
	resourceRegistry[name] = &ResourceHandler{
		Resolver:       resolver,
		ReportedFields: fields,
	}
}

func GetHandler(name compliance.ResourceKind) *ResourceHandler {
	return resourceRegistry[name]
}
