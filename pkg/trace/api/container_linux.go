// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"context"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics"
)

type ucredKey struct{}

// cacheExpiration determines how long a pid->container ID mapping is considered valid. This value is
// somewhat arbitrarily chosen, but just needs to be large enough to reduce latency and I/O load
// caused by frequently reading mappings, and small enough that pid-reuse doesn't cause mismatching
// of pids with container ids. A one minute cache means the latency and I/O should be low, and
// there would have to be thousands of containers spawned and dying per second to cause a mismatch.
const cacheExpiration = time.Minute

// connContext injects a Unix Domain Socket's User Credentials into the
// context.Context object provided. This is useful as the ConnContext member of an http.Server, to
// provide User Credentials to HTTP handlers.
//
// If the connection c is not a *net.UnixConn, the unchanged context is returned.
func connContext(ctx context.Context, c net.Conn) context.Context {
	s, ok := c.(*net.UnixConn)
	if !ok {
		return ctx
	}
	file, err := s.File()
	if err != nil {
		log.Debugf("Failed to obtain unix socket file: %v", err)
		return ctx
	}
	fd := int(file.Fd())
	ucred, err := syscall.GetsockoptUcred(fd, syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	if err != nil {
		log.Debugf("Failed to read credentials from unix socket: %v", err)
		return ctx
	}
	return context.WithValue(ctx, ucredKey{}, ucred)
}

// GetContainerID attempts first to read the container ID set by the client in the request header.
// If no such header is present or the value is empty, the function looks for a
// syscall.Ucred object in the context (see: connContext), determines the PID of the sender, and
// then uses the Meta Collector to map the PID to a container ID. If any of these fail, the
// function returns the empty string.
func GetContainerID(ctx context.Context, h http.Header) string {
	if id := h.Get(headerContainerID); id != "" {
		return id
	}
	ucred, ok := ctx.Value(ucredKey{}).(*syscall.Ucred)
	if !ok || ucred == nil {
		return ""
	}
	cid, err := metrics.GetProvider().GetMetaCollector().GetContainerIDForPID(int(ucred.Pid), cacheExpiration)
	if err != nil {
		log.Debugf("Could not get container ID from pid: %d: %v\n", ucred.Pid, err)
		return ""
	}
	return cid
}
