// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package backend

import "github.com/DataDog/datadog-secret-backend/secret"

// errorBackend links an error to its backend
type errorBackend struct {
	err string
}

// NewErrorBackend returns a new errorBackend
func NewErrorBackend(e error) Backend {
	return &errorBackend{err: e.Error()}
}

// GetSecretOutput returns a the value for a specific secret
func (b *errorBackend) GetSecretOutput(_ string) secret.Output {
	return secret.Output{
		Value: nil,
		Error: &b.err,
	}
}
