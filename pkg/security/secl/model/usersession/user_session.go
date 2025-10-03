// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package usersession holds model related to the user session context
package usersession

var (
	// UserSessionTypeUnknown is the unknown user session type
	UserSessionTypeUnknown Type = 0
	// UserSessionTypeK8S is the k8s user session type
	UserSessionTypeK8S Type = 1
	// UserSessionTypeSSH is the ssh user session type
	UserSessionTypeSSH Type = 2
)

var (
	// UserSessionTypes are the supported user session types
	UserSessionTypes = map[string]Type{
		"unknown": UserSessionTypeUnknown,
		"k8s":     UserSessionTypeK8S,
		"ssh":     UserSessionTypeSSH,
	}

	// UserSessionTypeStrings is used to
	UserSessionTypeStrings = map[Type]string{}
)

// Type is used to identify the User Session type
type Type uint8

func (ust Type) String() string {
	return UserSessionTypeStrings[ust]
}

// InitUserSessionTypes initializes internal structures for parsing Type values
func InitUserSessionTypes() {
	for k, v := range UserSessionTypes {
		UserSessionTypeStrings[v] = k
	}
}
