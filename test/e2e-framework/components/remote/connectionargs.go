// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
)

type connectionArgs struct {
	host pulumi.StringInput
	user string

	// ==== Optional ====
	privateKeyPath        string
	privateKeyPassword    string
	sshAgentPath          string
	port                  int
	dialErrorLimit        int
	perDialTimeoutSeconds int
}

type ConnectionOption = func(*connectionArgs) error

func buildConnectionArgs(host pulumi.StringInput, user string, options ...ConnectionOption) (*connectionArgs, error) {
	args := &connectionArgs{
		host:                  host,
		user:                  user,
		port:                  22,
		dialErrorLimit:        100,
		perDialTimeoutSeconds: 5,
	}
	return common.ApplyOption(args, options)
}

// WithPrivateKeyPath [optional] sets the path to the private key to use for the connection
func WithPrivateKeyPath(path string) ConnectionOption {
	return func(args *connectionArgs) error {
		args.privateKeyPath = path
		return nil
	}
}

// WithPrivateKeyPassword [optional] sets the password to use in case the private key is encrypted
func WithPrivateKeyPassword(password string) ConnectionOption {
	return func(args *connectionArgs) error {
		args.privateKeyPassword = password
		return nil
	}
}

// WithSSHAgentPath [optional] sets the path to the SSH Agent socket. Default to environment variable SSH_AUTH_SOCK if present.
func WithSSHAgentPath(path string) ConnectionOption {
	return func(args *connectionArgs) error {
		args.sshAgentPath = path
		return nil
	}
}

// WithPort [optional] sets the port to use for the connection. Default to 22.
func WithPort(port int) ConnectionOption {
	return func(args *connectionArgs) error {
		args.port = port
		return nil
	}
}

// WithDialErrorLimit [optional] sets the maximum dial attempts for the connection. Defaults to 100.
func WithDialErrorLimit(limit int) ConnectionOption {
	return func(args *connectionArgs) error {
		if limit > 0 {
			args.dialErrorLimit = limit
		}
		return nil
	}
}

// WithPerDialTimeoutSeconds [optional] sets the per dial timeout in seconds for the connection. Defaults to 5.
func WithPerDialTimeoutSeconds(seconds int) ConnectionOption {
	return func(args *connectionArgs) error {
		if seconds > 0 {
			args.perDialTimeoutSeconds = seconds
		}
		return nil
	}
}
