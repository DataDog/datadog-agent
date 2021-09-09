package aws

import (
	"context"
	// "fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	// log "github.com/sirupsen/logrus"
)

func NewAwsConfigFromBackendConfig(backendId string, backendConfig map[string]string) (
	*aws.Config, error) {

	/* add LoadDefaultConfig support for:
	- SharedConfigFiles
	- SharedCredentialFiles
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

	// StaticCredentials (aws_access_key_id & aws_secret_access_key)
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

	// SharedConfigProfile (aws_profile)
	if profile, ok := backendConfig["aws_profile"]; ok {
		options = append(options, config.WithSharedConfigProfile(profile))
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), options...)

	// sts:AssumeRole (aws_role_arn)
	if arn, ok := backendConfig["aws_role_arn"]; ok {
		creds := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), arn,
			func(o *stscreds.AssumeRoleOptions) {
				if eid, ok := backendConfig["aws_external_id"]; ok {
					o.ExternalID = &eid
				}
			},
		)

		cfg.Credentials = aws.NewCredentialsCache(creds)
	}

	return &cfg, err
}
