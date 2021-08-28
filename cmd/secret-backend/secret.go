package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
)

type Secret interface {
	GetSecretOutput(string) SecretOutput
}

type Secrets struct {
	Secrets map[string]Secret
}

func NewSecrets() Secrets {
	return Secrets{
		Secrets: make(map[string]Secret, 0),
	}
}

func (s *Secrets) InitSecret(config map[string]string, secretId string) {
	if _, ok := s.Secrets[secretId]; ok {
		return
	}

	if _, ok := config["type"]; !ok {
		log.WithField("secret", secretId).Error("missing secret type in configuration")
		s.Secrets[secretId] = &ErrorSecret{SecretId: secretId,
			Error: fmt.Errorf("missing secret type in configuration"),
		}
		return
	}

	secretType := config["type"]
	switch secretType {
	case "aws.secretsmanager":
		secret, err := NewAwsSecretsManagerSecret(config, secretId)
		if err != nil {
			s.Secrets[secretId] = &ErrorSecret{SecretId: secretId, Error: err}
		}
		s.Secrets[secretId] = secret
	default:
		log.WithFields(log.Fields{
			"secret":     secretId,
			"secretType": secretType,
		}).Error("unsupported secret type")
		s.Secrets[secretId] = &ErrorSecret{
			SecretId: secretId,
			Error:    fmt.Errorf("unsupported secret type: %s", secretType),
		}
	}

	return
}
