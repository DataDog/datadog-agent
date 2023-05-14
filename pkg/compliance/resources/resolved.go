// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
)

// ErrUnresolvedResource must be returned by a resource handler when it failed to resolve the resource
var ErrUnresolvedResource = errors.New("unresolved resource")

// Resolved maps an evaluation instance and a resource
type Resolved interface {
	InputType() string
}

// Unresolved must be returned by a resource handler when it failed to resolve the resource
type Unresolved struct {
	resourceType string
}

func (u *Unresolved) InputType() string {
	return u.resourceType
}

func NewUnresolvedInstance(resourceType string) *Unresolved {
	return &Unresolved{resourceType: resourceType}
}

// ResolvedInstance maps an evaluation instance and a resource
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

func (ri *resolvedInstance) InputType() string {
	return "object"
}

// NewResolvedInstance returns a new resolved instance
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

// InputType implements the Resolved interface
func (rr *resolvedIterator) InputType() string {
	return "array"
}

// NewResolvedIterator returns an iterator of resolved instances
func NewResolvedIterator(iterator eval.Iterator) *resolvedIterator {
	return &resolvedIterator{
		Iterator: iterator,
	}
}

// NewResolvedInstances returns an iterator from a list of resolved instances
func NewResolvedInstances(resolvedInstances []ResolvedInstance) *resolvedIterator {
	instances := make([]eval.Instance, len(resolvedInstances))
	for i, ri := range resolvedInstances {
		instances[i] = ri
	}
	return NewResolvedIterator(newInstanceIterator(instances))
}

// Resolver is a function to resolve a resource from its definition
type Resolver func(ctx context.Context, e env.Env, ruleID string, resource compliance.ResourceCommon, rego bool) (Resolved, error)
