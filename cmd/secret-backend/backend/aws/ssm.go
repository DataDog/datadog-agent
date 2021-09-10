package aws

import (
	"context"
	"errors"
	"strings"

	// "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/rapdev-io/datadog-secret-backend/secret"

	log "github.com/sirupsen/logrus"
)

type AwsSsmParameterStoreBackend struct {
	BackendId string
	Config    map[string]interface{}
	Secret    map[string]string
}

func NewAwsSsmParameterStoreBackend(backendId string, backendConfig map[string]interface{}) (
	*AwsSsmParameterStoreBackend, error) {

	secretValue := make(map[string]string, 0)

	cfg, err := NewAwsConfigFromBackendConfig(backendId, backendConfig)
	if err != nil {
		log.WithFields(log.Fields{
			"backend_id": backendId,
		}).WithError(err).Error("failed to initialize aws session")
		return nil, err
	}
	client := ssm.NewFromConfig(*cfg)

	// GetParametersByPath
	if path, ok := backendConfig["parameter_path"].(string); ok {
		input := &ssm.GetParametersByPathInput{
			Path:           &path,
			Recursive:      true,
			WithDecryption: true,
		}

		pager := ssm.NewGetParametersByPathPaginator(client, input)
		for pager.HasMorePages() {
			out, err := pager.NextPage(context.TODO())
			if err != nil {
				log.WithFields(log.Fields{
					"backend_id":        backendId,
					"backend_type":      backendConfig["backend_type"].(string),
					"parameter_path":    backendConfig["parameter_path"].(string),
					"aws_access_key_id": backendConfig["aws_access_key_id"].(string),
					"aws_profile":       backendConfig["aws_profile"].(string),
					"aws_region":        backendConfig["aws_region"].(string),
				}).WithError(err).Error("failed to retrieve parameters from path")
				return nil, err
			}

			for _, parameter := range out.Parameters {
				secretValue[*parameter.Name] = *parameter.Value
			}
		}
	}

	// GetParameters
	if inames, ok := backendConfig["parameters"].([]interface{}); ok {
		names := make([]string, 0)
		for _, iname := range inames {
			names = append(names, iname.(string))
		}

		input := &ssm.GetParametersInput{Names: names, WithDecryption: true}
		out, err := client.GetParameters(context.TODO(), input)
		if err != nil {
			log.WithFields(log.Fields{
				"backend_id":        backendId,
				"backend_type":      backendConfig["backend_type"].(string),
				"parameters":        strings.Join(names, ","),
				"aws_access_key_id": backendConfig["aws_access_key_id"].(string),
				"aws_profile":       backendConfig["aws_profile"].(string),
				"aws_region":        backendConfig["aws_region"].(string),
			}).WithError(err).Error("failed to retrieve parameters")
			return nil, err
		}

		// handle invalid parameters?
		for _, parameter := range out.Parameters {
			secretValue[*parameter.Name] = *parameter.Value
		}
	}

	backend := &AwsSsmParameterStoreBackend{
		BackendId: backendId,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

func (b *AwsSsmParameterStoreBackend) GetSecretOutput(secretKey string) secret.SecretOutput {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.SecretOutput{Value: &val, Error: nil}
	}
	es := errors.New("backend does not provide secret key").Error()

	log.WithFields(log.Fields{
		"backend_id":   b.BackendId,
		"backend_type": b.Config["backend_type"],
	}).Error("backend does not provide secret key")
	return secret.SecretOutput{Value: nil, Error: &es}
}
