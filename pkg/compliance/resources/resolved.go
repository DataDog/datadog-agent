// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
)

type Resolved interface {
}

type ResolvedInstance interface {
	eval.Instance
	ID() string
	Type() string
}

type resolvedInstance struct {
	eval.Instance
	id   string
	kind string
}

func (ri *resolvedInstance) ID() string {
	return ri.id
}

func (ri *resolvedInstance) Type() string {
	return ri.kind
}

func NewResolvedInstance(instance eval.Instance, resourceID, resourceType string) *resolvedInstance {
	return &resolvedInstance{
		Instance: instance,
		id:       resourceID,
		kind:     resourceType,
	}
}

type resolvedIterator struct {
	eval.Iterator
}

func NewResolvedIterator(iterator eval.Iterator) *resolvedIterator {
	return &resolvedIterator{
		Iterator: iterator,
	}
}

func NewResolvedInstances(resolvedInstances []ResolvedInstance) *resolvedIterator {
	instances := make([]eval.Instance, len(resolvedInstances))
	for i, ri := range resolvedInstances {
		instances[i] = ri
	}
	return NewResolvedIterator(newInstanceIterator(instances))
}

type Resolver func(ctx context.Context, e env.Env, ruleID string, resource compliance.ResourceCommon, rego bool) (Resolved, error)
