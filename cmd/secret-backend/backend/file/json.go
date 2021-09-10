package file

import (
	"encoding/json"
	"errors"
	"io/ioutil"

	"github.com/mitchellh/mapstructure"
	"github.com/rapdev-io/datadog-secret-backend/secret"

	log "github.com/sirupsen/logrus"
)

type FileJsonBackendConfig struct {
	BackendType string `mapstructure:"backend_type"`
	FilePath    string `mapstructure:"file_path"`
}

type FileJsonBackend struct {
	BackendId string
	Config    FileJsonBackendConfig
	Secret    map[string]string
}

func NewFileJsonBackend(backendId string, bc map[string]interface{}) (
	*FileJsonBackend, error) {

	backendConfig := FileJsonBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.WithError(err).Error("failed to map backend configuration")
		return nil, err
	}

	content, err := ioutil.ReadFile(backendConfig.FilePath)
	if err != nil {
		log.WithField("file_path", backendConfig.FilePath).
			WithError(err).Error("failed to read json secret file")
		return nil, err
	}

	secretValue := make(map[string]string, 0)
	if err := json.Unmarshal(content, &secretValue); err != nil {
		log.WithField("file_path", backendConfig.FilePath).
			WithError(err).Error("failed to unmarshal json secret")
		return nil, err
	}

	backend := &FileJsonBackend{
		BackendId: backendId,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

func (b *FileJsonBackend) GetSecretOutput(secretKey string) secret.SecretOutput {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.SecretOutput{Value: &val, Error: nil}
	}
	es := errors.New("backend does not provide secret key").Error()

	log.WithFields(log.Fields{
		"backend_id":   b.BackendId,
		"backend_type": b.Config.BackendType,
		"file_path":    b.Config.FilePath,
		"secret_key":   secretKey,
	}).Error("backend does not provide secret key")
	return secret.SecretOutput{Value: nil, Error: &es}
}
