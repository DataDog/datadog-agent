// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package backend

import "github.com/DataDog/datadog-secret-backend/secret"

// ErrorBackend links an error to its backend
type ErrorBackend struct {
	BackendID string
	Error     error
}

// NewErrorBackend returns a new ErrorBackend
func NewErrorBackend(backendID string, e error) *ErrorBackend {
	return &ErrorBackend{BackendID: backendID, Error: e}
}

// GetSecretOutput returns a the value for a specific secret
func (b *ErrorBackend) GetSecretOutput(_ string) secret.Output {
	es := b.Error.Error()
	return secret.Output{
		Value: nil,
		Error: &es,
	}
}
