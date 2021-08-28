package backend

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"strings"

	"github.com/rapdev-io/datadog-secret-backend/backend/aws"
	"github.com/rapdev-io/datadog-secret-backend/secret"

	log "github.com/sirupsen/logrus"
)

type Backend interface {
	GetSecretOutput(string) secret.SecretOutput
}

type Backends struct {
	Backends map[string]Backend
}

type BackendConfigurations struct {
	Configs map[string]map[string]string `yaml:"backends"`
}

func NewBackends(configFile *string) Backends {
	backends := Backends{
		Backends: make(map[string]Backend, 0),
	}

	configYAML, err := ioutil.ReadFile(*configFile)
	if err != nil {
		log.WithField("configFile", *configFile).
			WithError(err).Fatal("failed to configuration file")
	}

	backendConfigs := &BackendConfigurations{}
	if err := yaml.Unmarshal(configYAML, backendConfigs); err != nil {
		log.WithField("configFile", *configFile).
			WithError(err).Fatal("failed to unmarshal configuration yaml")
	}

	for k, v := range backendConfigs.Configs {
		backends.InitBackend(k, v)
	}

	return backends
}

func (b *Backends) InitBackend(backendId string, config map[string]string) {
	if _, ok := b.Backends[backendId]; ok {
		return
	}

	if _, ok := config["type"]; !ok {
		log.WithFields(log.Fields{
			"backendId": backendId,
		}).Error("undefined secret backend type in configuration")

		b.Backends[backendId] = &ErrorBackend{BackendId: backendId,
			Error: fmt.Errorf("undefined secret backend type in configuration"),
		}
		return
	}

	switch backendType := config["type"]; backendType {
	case "aws.secretsmanager":
		backend, err := aws.NewAwsSecretsManagerBackend(backendId, config)
		if err != nil {
			b.Backends[backendId] = &ErrorBackend{BackendId: backendId, Error: err}
		}
		b.Backends[backendId] = backend
	default:
		log.WithFields(log.Fields{
			"backendId":   backendId,
			"backendType": backendType,
		}).Error("unsupported backend type")

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
			log.WithFields(log.Fields{
				"backendId": backendId,
				"secretKey": secretKey,
			}).Error("undefined backend")

			b.Backends[backendId] = &ErrorBackend{
				BackendId: backendId,
				Error:     fmt.Errorf("undefined backend"),
			}
		}
		secretOutputs[s] = b.Backends[backendId].GetSecretOutput(secretKey)
	}
	return secretOutputs
}
