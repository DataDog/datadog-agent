package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	log "github.com/sirupsen/logrus"
)

func NewAwsConfigFromBackendConfig(backendId string, backendConfig map[string]string) (
	*aws.Config, error) {

	// TODO: implement validator with map dive

	// Static Credentials
	if _, ok := backendConfig["accessKeyId"]; ok {
		if _, ok := backendConfig["secretAccessKey"]; !ok {
			log.WithFields(log.Fields{
				"backendId": backendId,
			}).Errorf("missing required configuration parameter: %s", "secretAccessKey")
			return nil,
				fmt.Errorf("missing required configuration parameter: %s", "secretAccessKey")
		}
		return NewStaticCredentialsConfig(backendId, backendConfig)
	}

	// Profile Credentials
	if _, ok := backendConfig["awsProfile"]; ok {
		return NewProfileCredentialsConfig(backendId, backendConfig)
	}

	// Default Credentials
	return NewDefaultCredentialsConfig(backendId, backendConfig)
}

func NewStaticCredentialsConfig(backendId string, backendConfig map[string]string) (
	*aws.Config, error) {

	if _, ok := backendConfig["awsRegion"]; !ok {
		log.WithFields(log.Fields{
			"backendId": backendId,
		}).Errorf("missing required configuration parameter: %s", "awsRegion")
		return nil,
			fmt.Errorf("missing required configuration parameter: %s", "awsRegion")
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     backendConfig["accessKeyId"],
				SecretAccessKey: backendConfig["secretAccessKey"],
			},
		}),
		config.WithRegion(backendConfig["awsRegion"]),
	)
	if err != nil {
		log.WithFields(log.Fields{
			"backendId":   backendId,
			"backendType": backendConfig["type"],
			"awsRegion":   backendConfig["awsRegion"],
			"accessKeyId": backendConfig["accessKeyId"],
		}).WithError(err).Error("aws static credentials error")
		return nil, err
	}
	return &cfg, nil
}

func NewProfileCredentialsConfig(backendId string, backendConfig map[string]string) (
	*aws.Config, error) {

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithSharedConfigProfile(backendConfig["awsProfile"]),
	)
	if err != nil {
		log.WithFields(log.Fields{
			"backendId":   backendId,
			"backendType": backendConfig["type"],
			"awsProfile":  backendConfig["awsProfile"],
		}).WithError(err).Error("aws profile credentials error")
		return nil, err
	}
	return &cfg, nil
}

func NewDefaultCredentialsConfig(backendId string, backendConfig map[string]string) (
	*aws.Config, error) {

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.WithFields(log.Fields{
			"backendId":   backendId,
			"backendType": backendConfig["type"],
		}).WithError(err).Error("aws default credentials error")
		return nil, err
	}
	return &cfg, nil
}
