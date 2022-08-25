// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build !linux
// +build !linux

package api

import (
	"context"
	"net"
	"net/http"
)

// connContext is unimplemented for non-linux builds.
func connContext(ctx context.Context, c net.Conn) context.Context {
	return ctx
}

type IDProvider interface {
	GetContainerID(context.Context, http.Header) string
}

type CgroupIDProvider struct{}

func NewIDProvider(procRoot string) IDProvider {
	return &CgroupIDProvider{}
}

func (_ *CgroupIDProvider) GetContainerID(_ context.Context, h http.Header) string {
	return h.Get(headerContainerID)
}
