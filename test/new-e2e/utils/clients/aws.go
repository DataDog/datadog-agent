package clients

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

const (
	awsTimeout = 5 * time.Second
)

var (
	initLock     = sync.Mutex{}
	awsConfig    *aws.Config
	awsSSMClient *ssm.Client
)

func GetAWSSSMClient() (*ssm.Client, error) {
	initLock.Lock()
	defer initLock.Unlock()

	if awsSSMClient != nil {
		return awsSSMClient, nil
	}

	cfg, err := getAWSConfig()
	if err != nil {
		return nil, err
	}

	awsSSMClient = ssm.NewFromConfig(*cfg)
	return awsSSMClient, nil
}

func getAWSConfig() (*aws.Config, error) {
	if awsConfig != nil {
		return awsConfig, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	awsConfig = &cfg
	return awsConfig, nil
}
