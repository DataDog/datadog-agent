// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package usersession holds model related to the user session context
package usersession

var (
	// UserSessionTypes are the supported user session types
	UserSessionTypes = map[string]Type{
		"unknown": 0,
		"k8s":     1,
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
