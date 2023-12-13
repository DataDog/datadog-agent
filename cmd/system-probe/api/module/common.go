// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"errors"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

// ErrNotEnabled is a special error type that should be returned by a Factory
// when the associated Module is not enabled.
var ErrNotEnabled = errors.New("module is not enabled")

// Factory encapsulates the initialization of a Module
type Factory struct {
	Name             config.ModuleName
	ConfigNamespaces []string
	Fn               func(cfg *config.Config) (Module, error)
}

// Module defines the common API implemented by every System Probe Module
type Module interface {
	GetStats() map[string]interface{}
	Register(*Router) error
	RegisterGRPC(grpc.ServiceRegistrar) error
	Close()
}
