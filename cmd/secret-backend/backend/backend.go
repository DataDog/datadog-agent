package backend

import (
	"fmt"
	"github.com/rapdev-io/datadog-secret-backend/backend/akeyless"
	"github.com/rapdev-io/datadog-secret-backend/backend/hashicorp"
	"io/ioutil"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"

	"github.com/rapdev-io/datadog-secret-backend/backend/aws"
	"github.com/rapdev-io/datadog-secret-backend/backend/azure"
	"github.com/rapdev-io/datadog-secret-backend/backend/file"
	"github.com/rapdev-io/datadog-secret-backend/secret"
)

type Backend interface {
	GetSecretOutput(string) secret.SecretOutput
}

type Backends struct {
	Backends map[string]Backend
}

type BackendConfigurations struct {
	Configs map[string]map[string]interface{} `yaml:"backends"`
}

func NewBackends(configFile *string) Backends {
	backends := Backends{
		Backends: make(map[string]Backend, 0),
	}

	configYAML, err := ioutil.ReadFile(*configFile)
	if err != nil {
		log.Fatal().Err(err).Str("config_file", *configFile).
			Msg("failed to read configuration file")
	}

	backendConfigs := &BackendConfigurations{}
	if err := yaml.Unmarshal(configYAML, backendConfigs); err != nil {
		log.Fatal().Err(err).Str("config_file", *configFile).
			Msg("failed to unmarshal configuration yaml")
	}

	for k, v := range backendConfigs.Configs {
		backends.InitBackend(k, v)
	}

	return backends
}

func (b *Backends) InitBackend(backendId string, config map[string]interface{}) {
	if _, ok := b.Backends[backendId]; ok {
		return
	}

	if _, ok := config["backend_type"].(string); !ok {
		log.Error().Str("backend_id", backendId).
			Msg("undefined secret backend type in configuration")

		b.Backends[backendId] = &ErrorBackend{
			BackendId: backendId,
			Error:     fmt.Errorf("undefined secret backend type in configuration"),
		}
		return
	}

	switch backendType := config["backend_type"].(string); backendType {
	case "aws.secrets":
		backend, err := aws.NewAwsSecretsManagerBackend(backendId, config)
		if err != nil {
			b.Backends[backendId] = NewErrorBackend(backendId, err)
		} else {
			b.Backends[backendId] = backend
		}
	case "aws.ssm":
		backend, err := aws.NewAwsSsmParameterStoreBackend(backendId, config)
		if err != nil {
			b.Backends[backendId] = NewErrorBackend(backendId, err)
		} else {
			b.Backends[backendId] = backend
		}
	case "azure.keyvault":
		backend, err := azure.NewAzureKeyVaultBackend(backendId, config)
		if err != nil {
			b.Backends[backendId] = NewErrorBackend(backendId, err)
		} else {
			b.Backends[backendId] = backend
		}
	case "hashicorp.vault":
		backend, err := hashicorp.NewVaultBackend(backendId, config)
		if err != nil {
			b.Backends[backendId] = NewErrorBackend(backendId, err)
		} else {
			b.Backends[backendId] = backend
		}
	case "file.yaml":
		backend, err := file.NewFileYamlBackend(backendId, config)
		if err != nil {
			b.Backends[backendId] = NewErrorBackend(backendId, err)
		} else {
			b.Backends[backendId] = backend
		}
	case "file.json":
		backend, err := file.NewFileJsonBackend(backendId, config)
		if err != nil {
			b.Backends[backendId] = NewErrorBackend(backendId, err)
		} else {
			b.Backends[backendId] = backend
		}
	case "akeyless":
		backend, err := akeyless.NewAkeylessBackend(backendId, config)
		if err != nil {
			b.Backends[backendId] = NewErrorBackend(backendId, err)
		} else {
			b.Backends[backendId] = backend
		}
	default:
		log.Error().Str("backend_id", backendId).Str("backend_type", backendType).
			Msg("unsupported backend type")

		b.Backends[backendId] = &ErrorBackend{
			BackendId: backendId,
			Error:     fmt.Errorf("unsupported backend type: %s", backendType),
		}
	}
	return
}

func (b *Backends) GetSecretOutputs(secrets []string) map[string]secret.SecretOutput {
	secretOutputs := make(map[string]secret.SecretOutput, 0)

	for _, s := range secrets {
		segments := strings.SplitN(s, ":", 2)
		backendId := segments[0]
		secretKey := segments[1]

		if _, ok := b.Backends[backendId]; !ok {
			log.Error().Str("backend_id", backendId).Str("secret_key", secretKey).
				Msg("undefined backend")

			b.Backends[backendId] = &ErrorBackend{
				BackendId: backendId,
				Error:     fmt.Errorf("undefined backend"),
			}
		}
		secretOutputs[s] = b.Backends[backendId].GetSecretOutput(secretKey)
	}
	return secretOutputs
}
