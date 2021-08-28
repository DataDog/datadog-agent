package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	log "github.com/sirupsen/logrus"
)

func NewAwsConfigFromBackendConfig(backendId string, backendConfig map[string]string,
	awsRegion string) (*aws.Config, error) {

	if _, ok := backendConfig["accessKeyId"]; ok {
		if _, ok := backendConfig["secretAccessKey"]; !ok {
			log.WithFields(log.Fields{
				"backendId": backendId,
			}).Errorf("missing required configuration parameter: %s", "secretAccessKey")
			return nil,
				fmt.Errorf("missing required configuration parameter: %s", "secretAccessKey")
		}
		return NewStaticCredentialsConfig(backendId, backendConfig, awsRegion)
	}

	return nil, fmt.Errorf("no backend configuration aws session defined")
}

func NewStaticCredentialsConfig(backendId string, backendConfig map[string]string,
	awsRegion string) (*aws.Config, error) {

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     backendConfig["accessKeyId"],
				SecretAccessKey: backendConfig["secretAccessKey"],
			},
		}),
		config.WithRegion(awsRegion),
	)
	if err != nil {
		log.WithFields(log.Fields{
			"backendId":   backendId,
			"backendType": backendConfig["type"],
			"awsRegion":   awsRegion,
			"accessKeyId": backendConfig["accessKeyId"],
		}).WithError(err).Error("aws static credentials error")
		return nil, err
	}
	return &cfg, nil
}

// func NewProfileCredentials() {}

// func NewDefaultCredentails() {}
