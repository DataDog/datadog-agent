// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

package secrets

// SecretVal defines the structure for secrets in JSON output
type SecretVal struct {
	Value    string `json:"value,omitempty"`
	ErrorMsg string `json:"error,omitempty"`
}

// ResolveCallback is the callback type used by Subscribe method to send notifications
// This callback will be called once for each time a handle shows up at a particular path
type ResolveCallback func(handle string, path []string, oldValue, newValue any)

// PayloadVersion defines the current payload version sent to a secret backend
const PayloadVersion = "1.0"
