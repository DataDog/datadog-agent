// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package noneimpl implements the grpcserver component interface
// It does not create a grpc server nor a gateway
package noneimpl

import (
	"net/http"

	grpc "github.com/DataDog/datadog-agent/comp/api/grpcserver/def"
)

type server struct {
}

func (s *server) BuildServer() http.Handler {
	return nil
}

// Provides defines the output of the grpc component
type Provides struct {
	Comp grpc.Component
}

// NewComponent creates a new grpc component
func NewComponent() (Provides, error) {
	provides := Provides{
		Comp: &server{},
	}
	return provides, nil
}
