// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build !serverless

package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"syscall"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// cgroupV1BaseController is the name of the cgroup controller used to parse /proc/<pid>/cgroup
const cgroupV1BaseController = "memory"

type ucredKey struct{}

// connContext injects a Unix Domain Socket's User Credentials into the
// context.Context object provided. This is useful as the connContext member of an http.Server, to
// provide User Credentials to HTTP handlers.
//
// If the connection c is not a *net.UnixConn, the unchanged context is returned.
func connContext(ctx context.Context, c net.Conn) context.Context {
	if oc, ok := c.(*onCloseConn); ok {
		c = oc.Conn
	}
	s, ok := c.(*net.UnixConn)
	if !ok {
		return ctx
	}
	raw, err := s.SyscallConn()
	if err != nil {
		log.Debugf("Failed to read credentials from unix socket: %v", err)
		return ctx
	}
	var (
		ucred *syscall.Ucred
		cerr  error
	)
	err = raw.Control(func(fd uintptr) {
		ucred, cerr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		log.Debugf("Failed to control raw unix socket: %v", err)
		return ctx
	}
	if cerr != nil {
		log.Debugf("Failed to read credentials from unix socket: %v", cerr)
		return ctx
	}

	return context.WithValue(ctx, ucredKey{}, ucred)
}

// IDProvider implementations are able to look up a container ID given a ctx and http header.
type IDProvider interface {
	GetContainerID(context.Context, http.Header) string
}

// noCgroupsProvider is a fallback IDProvider that only looks in the http header for a container ID.
type noCgroupsProvider struct{}

func (i *noCgroupsProvider) GetContainerID(_ context.Context, h http.Header) string {
	return h.Get(header.ContainerID)
}

// NewIDProvider initializes an IDProvider instance using the provided procRoot to perform cgroups lookups in linux environments.
func NewIDProvider(procRoot string, containerIDFromOriginInfo func(originInfo origindetection.OriginInfo) (string, error)) IDProvider {
	// taken from pkg/util/containers/metrics/system.collector_linux.go
	var hostPrefix string
	if strings.HasPrefix(procRoot, "/host") {
		hostPrefix = "/host"
	}

	reader, err := cgroups.NewReader(
		cgroups.WithCgroupV1BaseController(cgroupV1BaseController),
		cgroups.WithProcPath(procRoot),
		cgroups.WithHostPrefix(hostPrefix),
		cgroups.WithReaderFilter(cgroups.ContainerFilter), // Will parse the path in /proc/<pid>/cgroup to get the container ID.
	)

	if err != nil {
		log.Warnf("Failed to identify cgroups version due to err: %v. APM data may be missing containerIDs for applications running in containers. This will prevent spans from being associated with container tags.", err)
		return &noCgroupsProvider{}
	}
	cgroupController := ""
	if reader.CgroupVersion() == 1 {
		cgroupController = cgroupV1BaseController // The 'memory' controller is used by the cgroupv1 utils in the agent to parse the procfs.
	}
	return &cgroupIDProvider{
		procRoot:                  procRoot,
		controller:                cgroupController,
		reader:                    reader,
		containerIDFromOriginInfo: containerIDFromOriginInfo,
	}
}

type cgroupIDProvider struct {
	procRoot   string
	controller string
	// reader is used to retrieve the container ID from its cgroup v2 inode.
	reader                    *cgroups.Reader
	containerIDFromOriginInfo func(originInfo origindetection.OriginInfo) (string, error)
}

// GetContainerID retrieves the container ID associated with the given request.
//
// The container ID can be determined from multiple sources in the following order:
//  1. **Local Data Header** (`LocalData`): If present, it is parsed to extract the container ID or inode.
//     If an inode is found instead of a container ID, it is resolved to a container ID.
//  2. **Datadog-Container-ID Header**: A deprecated fallback used for backward compatibility.
//  3. **Process Context (PID)**: If no container ID is found in headers, the function attempts
//     to resolve it using the PID from the provided context, checking cgroups.
//  4. **External Data Header** (`ExternalData`): If present, it is parsed as an additional source.
//
// If none of the direct methods return a valid container ID, an attempt is made to generate one
// based on the collected OriginInfo.
func (c *cgroupIDProvider) GetContainerID(ctx context.Context, h http.Header) string {
	originInfo := origindetection.OriginInfo{ProductOrigin: origindetection.ProductOriginAPM}

	// Parse LocalData from the headers.
	if localData := h.Get(header.LocalData); localData != "" {
		var err error
		originInfo.LocalData, err = origindetection.ParseLocalData(localData)
		if err != nil {
			log.Errorf("Could not parse local data (%s): %v", localData, err)
		}

		if originInfo.LocalData.ContainerID != "" {
			return originInfo.LocalData.ContainerID
		}
	}

	// Retrieve container ID from Datadog-Container-ID header.
	// Deprecated in favor of LocalData header. This is kept for backward compatibility with older libraries.
	if containerIDFromHeader := h.Get(header.ContainerID); containerIDFromHeader != "" {
		return containerIDFromHeader
	}

	// Retrieve the PID from the context.
	ucred, ok := ctx.Value(ucredKey{}).(*syscall.Ucred)
	if !ok || ucred == nil {
		log.Debugf("Could not retrieve PID from context")
	} else {
		originInfo.LocalData.ProcessID = uint32(ucred.Pid)
	}

	// Parse ExternalData from the headers.
	if externalData := h.Get(header.ExternalData); externalData != "" {
		var err error
		originInfo.ExternalData, err = origindetection.ParseExternalData(externalData)
		if err != nil {
			log.Errorf("Could not parse external data (%s): %v", externalData, err)
		}
	}

	// Generate container ID from OriginInfo.
	generatedContainerID, err := c.containerIDFromOriginInfo(originInfo)
	if err != nil {
		log.Debugf("Could not generate container ID from OriginInfo: %+v, err: %v", originInfo, err)
	}
	return generatedContainerID
}
