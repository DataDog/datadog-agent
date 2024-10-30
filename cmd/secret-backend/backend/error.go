// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package backend

import "github.com/rapdev-io/datadog-secret-backend/secret"

type ErrorBackend struct {
	BackendId string
	Error     error
}

func NewErrorBackend(backendId string, e error) *ErrorBackend {
	return &ErrorBackend{BackendId: backendId, Error: e}
}

func (b *ErrorBackend) GetSecretOutput(secretKey string) secret.SecretOutput {
	es := b.Error.Error()
	return secret.SecretOutput{
		Value: nil,
		Error: &es,
	}
}
