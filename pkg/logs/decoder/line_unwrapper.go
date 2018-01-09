// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package decoder

import (
	"bytes"
)

// LineUnwrapper removes all the extra information that were added to the original log
type LineUnwrapper interface {
	Unwrap(line []byte) []byte
}

// IdentityUnwrapper does nothing
type identityUnwrapper struct{}

// Unwrap returns line
func (u identityUnwrapper) Unwrap(line []byte) []byte {
	return line
}

// NewUnwrapper returns a default LineUnwrapper that does nothing
func NewUnwrapper() LineUnwrapper {
	return &identityUnwrapper{}
}

// headerLen represents the length of the header of docker logs
// see here for more information: https://godoc.org/github.com/moby/moby/client#Client.ContainerLogs
const headerLen = 8

// DockerUnwrapper removes all the information added by docker to logs coming from containers
type DockerUnwrapper struct{}

// NewDockerUnwrapper returns a new DockerUnwrapper
func NewDockerUnwrapper() *DockerUnwrapper {
	return &DockerUnwrapper{}
}

// Unwrap removes the header and the timestamp from container logs
func (u DockerUnwrapper) Unwrap(line []byte) []byte {
	if len(line) < headerLen {
		return line
	}
	to := bytes.Index(line[headerLen:], []byte{' '}) // skip the timestamp
	if to == -1 {
		return line
	}
	to += headerLen // skip the header
	return line[to+1:]
}
