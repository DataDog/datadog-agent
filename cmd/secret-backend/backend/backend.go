// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package backend aggregates all supported backends and allow fetching secrets from them
package backend

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-secret-backend/backend/akeyless"
	"github.com/DataDog/datadog-secret-backend/backend/aws"
	"github.com/DataDog/datadog-secret-backend/backend/azure"
	"github.com/DataDog/datadog-secret-backend/backend/file"
	"github.com/DataDog/datadog-secret-backend/backend/hashicorp"
	"github.com/DataDog/datadog-secret-backend/secret"
)

// Backend represents the common interface for all secret backends
type Backend interface {
	GetSecretOutput(string) secret.Output
}

// Backends encapsulate all known backends
type Backends struct {
	Backends map[string]Backend
}

type configurations struct {
	Configs map[string]map[string]interface{} `yaml:"backends"`
}

// NewBackends returns a new Backends
func NewBackends(configFile string) Backends {
	backends := Backends{
		Backends: make(map[string]Backend, 0),
	}

	configYAML, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatal().Err(err).Str("config_file", configFile).
			Msg("failed to read configuration file")
	}

	backendConfigs := &configurations{}
	if err := yaml.Unmarshal(configYAML, backendConfigs); err != nil {
		log.Fatal().Err(err).Str("config_file", configFile).
			Msg("failed to unmarshal configuration yaml")
	}

	for k, v := range backendConfigs.Configs {
		backends.InitBackend(k, v)
	}

	return backends
}

// InitBackend initialize all the backends based on their configuration
func (b *Backends) InitBackend(backendID string, config map[string]interface{}) {
	if _, ok := b.Backends[backendID]; ok {
		return
	}

	if _, ok := config["backend_type"].(string); !ok {
		log.Error().Str("backend_id", backendID).
			Msg("undefined secret backend type in configuration")

		b.Backends[backendID] = &ErrorBackend{
			BackendID: backendID,
			Error:     fmt.Errorf("undefined secret backend type in configuration"),
		}
		return
	}

	switch backendType := config["backend_type"].(string); backendType {
	case "aws.secrets":
		backend, err := aws.NewSecretsManagerBackend(backendID, config)
		if err != nil {
			b.Backends[backendID] = NewErrorBackend(backendID, err)
		} else {
			b.Backends[backendID] = backend
		}
	case "aws.ssm":
		backend, err := aws.NewSSMParameterStoreBackend(backendID, config)
		if err != nil {
			b.Backends[backendID] = NewErrorBackend(backendID, err)
		} else {
			b.Backends[backendID] = backend
		}
	case "azure.keyvault":
		backend, err := azure.NewKeyVaultBackend(backendID, config)
		if err != nil {
			b.Backends[backendID] = NewErrorBackend(backendID, err)
		} else {
			b.Backends[backendID] = backend
		}
	case "hashicorp.vault":
		backend, err := hashicorp.NewVaultBackend(backendID, config)
		if err != nil {
			b.Backends[backendID] = NewErrorBackend(backendID, err)
		} else {
			b.Backends[backendID] = backend
		}
	case "file.yaml":
		backend, err := file.NewYAMLBackend(backendID, config)
		if err != nil {
			b.Backends[backendID] = NewErrorBackend(backendID, err)
		} else {
			b.Backends[backendID] = backend
		}
	case "file.json":
		backend, err := file.NewJSONBackend(backendID, config)
		if err != nil {
			b.Backends[backendID] = NewErrorBackend(backendID, err)
		} else {
			b.Backends[backendID] = backend
		}
	case "akeyless":
		backend, err := akeyless.NewAkeylessBackend(backendID, config)
		if err != nil {
			b.Backends[backendID] = NewErrorBackend(backendID, err)
		} else {
			b.Backends[backendID] = backend
		}
	default:
		log.Error().Str("backend_id", backendID).Str("backend_type", backendType).
			Msg("unsupported backend type")

		b.Backends[backendID] = &ErrorBackend{
			BackendID: backendID,
			Error:     fmt.Errorf("unsupported backend type: %s", backendType),
		}
	}
}

// GetSecretOutputs returns a the value for a list of given secrets of form "<backendID>:<secret key>"
func (b *Backends) GetSecretOutputs(secrets []string) map[string]secret.Output {
	secretOutputs := make(map[string]secret.Output, 0)

	for _, s := range secrets {
		segments := strings.SplitN(s, ":", 2)
		backendID := segments[0]
		secretKey := segments[1]

		if _, ok := b.Backends[backendID]; !ok {
			log.Error().Str("backend_id", backendID).Str("secret_key", secretKey).
				Msg("undefined backend")

			b.Backends[backendID] = &ErrorBackend{
				BackendID: backendID,
				Error:     fmt.Errorf("undefined backend"),
			}
		}
		secretOutputs[s] = b.Backends[backendID].GetSecretOutput(secretKey)
	}
	return secretOutputs
}
