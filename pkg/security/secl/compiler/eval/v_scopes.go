// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

// VariableScope represents the scope of a variable
type VariableScope interface {
	Key() (string, bool)
	ParentScope() (VariableScope, bool)
}

// ReleasableVariableScope represents a scope that can be released
type ReleasableVariableScope interface {
	AppendReleaseCallback(callback func())
}

// ScoperFnc is the signature of variable scoper callback
type ScoperFnc func(ctx *Context) (VariableScope, error)

// VariableScoper represents a variable scoper
type VariableScoper struct {
	scoperType InternalScoperType
	getScopeCb ScoperFnc
}

// NewVariableScoper returns a new variable scoper
func NewVariableScoper(scoperType InternalScoperType, cb ScoperFnc) *VariableScoper {
	return &VariableScoper{
		scoperType: scoperType,
		getScopeCb: cb,
	}
}

// Type returns the type of the variable scoper
func (vs *VariableScoper) Type() InternalScoperType {
	return vs.scoperType
}

// GetScope returns a variable scope based on the given Context
func (vs *VariableScoper) GetScope(ctx *Context) (VariableScope, error) {
	return vs.getScopeCb(ctx)
}

// InternalScoperType represents the type of a scoper
type InternalScoperType int

const (
	// UndefinedScoperType is the undefinied scoper
	UndefinedScoperType InternalScoperType = iota
	// GlobalScoperType handles the global scope
	GlobalScoperType
	// ProcessScoperType handles process scopes
	ProcessScoperType
	// ContainerScoperType handles container scopes
	ContainerScoperType
	// CGroupScoperType handles cgroup scopes
	CGroupScoperType
)

// String returns the name of the scoper
func (isn InternalScoperType) String() string {
	switch isn {
	case GlobalScoperType:
		return "global"
	case ProcessScoperType:
		return "process"
	case ContainerScoperType:
		return "container"
	case CGroupScoperType:
		return "cgroup"
	default:
		return ""
	}
}

// VariablePrefix returns the variable prefix that corresponds to this scoper type
func (isn InternalScoperType) VariablePrefix() string {
	switch isn {
	case ProcessScoperType:
		return "process"
	case ContainerScoperType:
		return "container"
	case CGroupScoperType:
		return "cgroup"
	default:
		return ""
	}
}
