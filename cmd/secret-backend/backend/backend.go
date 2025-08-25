// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package backend aggregates all supported backends and allow fetching secrets from them
package backend

import (
	"fmt"

	"github.com/DataDog/datadog-secret-backend/backend/akeyless"
	"github.com/DataDog/datadog-secret-backend/backend/aws"
	"github.com/DataDog/datadog-secret-backend/backend/azure"
	"github.com/DataDog/datadog-secret-backend/backend/file"
	"github.com/DataDog/datadog-secret-backend/backend/hashicorp"
	"github.com/DataDog/datadog-secret-backend/secret"
)

// Backend represents the common interface for the secret backends
type Backend interface {
	GetSecretOutput(string) secret.Output
}

// Get initialize and return the requested backend
func Get(backendType string, backendConfig map[string]interface{}) Backend {
	var backend Backend
	var err error

	switch backendType {
	case "aws.secrets":
		backend, err = aws.NewSecretsManagerBackend(backendConfig)
	case "aws.ssm":
		backend, err = aws.NewSSMParameterStoreBackend(backendConfig)
	case "azure.keyvault":
		backend, err = azure.NewKeyVaultBackend(backendConfig)
	case "hashicorp.vault":
		backend, err = hashicorp.NewVaultBackend(backendConfig)
	case "file.yaml":
		backend, err = file.NewYAMLBackend(backendConfig)
	case "file.json":
		backend, err = file.NewJSONBackend(backendConfig)
	case "akeyless":
		backend, err = akeyless.NewAkeylessBackend(backendConfig)
	default:
		err = fmt.Errorf("unsupported backend type: %s", backendType)
	}
	if err != nil {
		return NewErrorBackend(err)
	}
	return backend
}
