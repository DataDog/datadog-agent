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

// GenericConnector encapsulate all known backends
type GenericConnector struct {
	Backend Backend
}

// InitBackend initialize the backend based on their configuration
func (g *GenericConnector) InitBackend(backendType string, backendConfig map[string]interface{}, backendSecrets []string) {
	backendConfig["backend_type"] = backendType
	switch backendType {
	case "aws.secrets":
		backend, err := aws.NewSecretsManagerBackend(backendConfig)
		if err != nil {
			g.Backend = NewErrorBackend(err)
		} else {
			g.Backend = backend
		}
	case "aws.ssm":
		backend, err := aws.NewSSMParameterStoreBackend(backendConfig)
		if err != nil {
			g.Backend = NewErrorBackend(err)
		} else {
			g.Backend = backend
		}
	case "azure.keyvault":
		backend, err := azure.NewKeyVaultBackend(backendConfig)
		if err != nil {
			g.Backend = NewErrorBackend(err)
		} else {
			g.Backend = backend
		}
	case "hashicorp.vault":
		backend, err := hashicorp.NewVaultBackend(backendConfig, backendSecrets)
		if err != nil {
			g.Backend = NewErrorBackend(err)
		} else {
			g.Backend = backend
		}
	case "file.yaml":
		backend, err := file.NewYAMLBackend(backendConfig)
		if err != nil {
			g.Backend = NewErrorBackend(err)
		} else {
			g.Backend = backend
		}
	case "file.json":
		backend, err := file.NewJSONBackend(backendConfig)
		if err != nil {
			g.Backend = NewErrorBackend(err)
		} else {
			g.Backend = backend
		}
	case "akeyless":
		backend, err := akeyless.NewAkeylessBackend(backendConfig)
		if err != nil {
			g.Backend = NewErrorBackend(err)
		} else {
			g.Backend = backend
		}
	default:
		g.Backend = &ErrorBackend{
			Error: fmt.Errorf("unsupported backend type: %s", backendType),
		}
	}
}

// GetSecretOutputs returns a the value for a list of given secrets of form "<secret key>"
func (g *GenericConnector) GetSecretOutputs(secrets []string) map[string]secret.Output {
	secretOutputs := make(map[string]secret.Output, 0)
	for _, secretKey := range secrets {
		secretOutputs[secretKey] = g.Backend.GetSecretOutput(secretKey)
	}
	return secretOutputs
}
