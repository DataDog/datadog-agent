// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

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

const cacheExpire = 5 * time.Minute

// connContext is a function that injects a Unix Domain Socket's User Credentials into the
// context.Context object provided. This is useful as the ConnContext member of an http.Server, to
// provide User Credentials to HTTP handlers.
func connContext(ctx context.Context, c net.Conn) context.Context {
	s, ok := c.(*net.UnixConn)
	if !ok {
		return ctx
	}
	file, err := s.File()
	if err != nil {
		return ctx
	}
	fd := int(file.Fd())
	acct, err := syscall.GetsockoptUcred(fd, syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	ctx = context.WithValue(ctx, ucredKey{}, acct)
	return ctx
}

// getContainerID attempts first to read the container ID set by the client in the request header.
// If no such header is present or the value is empty, it will look in the container ID cache. If
// there is a valid (not stale) container ID for the given pid, that is returned. Otherwise the
// container ID is parsed using readContainerID. If none of these methods succeed, getContainerID
// returns an empty string.
func getContainerID(req *http.Request) string {
	if id := req.Header.Get(headerContainerID); id != "" {
		return id
	}
	ucred, ok := req.Context().Value(ucredKey{}).(*syscall.Ucred)
	if !ok || ucred == nil {
		return ""
	}
	cid, err := metrics.GetProvider().GetMetaCollector().GetContainerIDForPID(int(ucred.Pid), cacheExpire)
	if err != nil {
		log.Debugf("Could not get credentials from provider: %v\n", err)
		return ""
	}
	return cid
}
