package aws

import (
	"context"
	// "fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	// log "github.com/sirupsen/logrus"
)

func NewAwsConfigFromBackendConfig(backendId string, backendConfig map[string]string) (
	*aws.Config, error) {

	/* add LoadDefaultConfig support for:
	- SharedConfigFiles
	- SharedCredentialFiles
	- sts:AssumeRole
	*/

	// build slice of LoadOptionsFunc for LoadDefaultConfig overrides
	options := make([]func(*config.LoadOptions) error, 0)

	// aws_region
	if region, ok := backendConfig["aws_region"]; ok {
		options = append(options, func(o *config.LoadOptions) error {
			o.Region = region
			return nil
		})
	}

	// aws_access_key_id & aws_secret_access_key
	if access_key, ok := backendConfig["aws_access_key_id"]; ok {
		if secret_key, ok := backendConfig["aws_secret_access_key"]; ok {
			options = append(options, func(o *config.LoadOptions) error {
				o.Credentials = credentials.StaticCredentialsProvider{
					Value: aws.Credentials{
						AccessKeyID:     access_key,
						SecretAccessKey: secret_key,
					},
				}
				return nil
			})
		}
	}

	// aws_profile
	if profile, ok := backendConfig["aws_profile"]; ok {
		options = append(options, config.WithSharedConfigProfile(profile))
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), options...)
	return &cfg, err
}
