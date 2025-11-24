// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package usersession holds model related to the user session context
package usersession

const (
	// UserSessionTypeUnknown is the unknown user session type
	UserSessionTypeUnknown Type = iota
	// UserSessionTypeK8S is the k8s user session type
	UserSessionTypeK8S
	// UserSessionTypeSSH is the ssh user session type
	UserSessionTypeSSH
)

// SSHAuthMethodConstants are the supported SSH authentication methods
const (
	// SSHAuthMethodUnknown is the unknown SSH authentication method
	SSHAuthMethodUnknown AuthType = iota
	// SSHAuthMethodPassword is the password SSH authentication method
	SSHAuthMethodPassword
	// SSHAuthMethodPublicKey is the public key SSH authentication method
	SSHAuthMethodPublicKey
)

// Type is used to identify the User Session type
type Type uint8

// AuthType is used to identify the SSH authentication method
type AuthType uint8
