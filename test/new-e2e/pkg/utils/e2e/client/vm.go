// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/executeparams"
)

// VM is an interface that provides methods to run commands on a virtual machine.
type VM interface {
	// ExecuteWithError executes a command and returns an error if any.
	ExecuteWithError(command string, options ...executeparams.Option) (string, error)

	// Execute executes a command and returns its output.
	Execute(command string, options ...executeparams.Option) string

	// CopyFile copy file to the remote host
	CopyFile(src string, dst string)

	// CopyFolder copy a folder to the remote host
	CopyFolder(srcFolder string, dstFolder string)
}
