// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/DataDog/datadog-agent/comp/core/secrets"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/fips"
	datadogHttp "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// secretsManagerClient is an interface that defines the methods we use from the ssm client
// As the AWS SDK doesn't provide a real mock, we'll have to make our own that
// matches this interface
type secretsManagerClient interface {
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// getSecretsManagerClient is a variable that holds the function to create a new secretsManagerClient
// it will be overwritten in tests
var getSecretsManagerClient = func(cfg aws.Config) secretsManagerClient {
	return secretsmanager.NewFromConfig(cfg)
}

// ReadAwsSecretsManagerSecret reads a secret stored in AWS Secrets Manager
func ReadAwsSecretsManagerSecret(id string) secrets.SecretVal {
	parsedArn, err := arn.Parse(id)
	if err != nil {
		return secrets.SecretVal{ErrorMsg: "Invalid format. Use: \"arn:aws:secretsmanager:<REGION>:<ACCOUNT_ID>:secret:<SECRET_NAME>\""}
	}

	shouldUseFips, err := fips.Enabled()
	if err != nil {
		log.Debugf("Could not determine if FIPS is enabled, assuming it is not: %v", err)
		shouldUseFips = false
	}

	fipsEndpointState := aws.FIPSEndpointStateUnset
	if shouldUseFips {
		fipsEndpointState = aws.FIPSEndpointStateEnabled
		log.Debug("Using FIPS endpoints for secrets management.")
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithHTTPClient(&http.Client{
			Transport: datadogHttp.CreateHTTPTransport(pkgconfigsetup.Datadog()),
		}),
		awsconfig.WithRegion(parsedArn.Region),
		awsconfig.WithUseFIPSEndpoint(fipsEndpointState),
	)
	if err != nil {
		return secrets.SecretVal{ErrorMsg: fmt.Sprintf("Unable to load aws configuration: %v", err)}
	}

	secretsManagerClient := getSecretsManagerClient(cfg)

	secret := &secretsmanager.GetSecretValueInput{
		SecretId: &id,
	}

	output, err := secretsManagerClient.GetSecretValue(context.TODO(), secret)
	if err != nil {
		return secrets.SecretVal{ErrorMsg: fmt.Sprintf("Secrets Manager read error: %v", err)}
	}

	if output.SecretString != nil {
		secretString := *output.SecretString // create a copy to not return an object within the AWS response
		return secrets.SecretVal{Value: secretString}
	} else if output.SecretBinary != nil {
		decodedBinarySecretBytes := make([]byte, base64.StdEncoding.DecodedLen(len(output.SecretBinary)))
		secretLen, err := base64.StdEncoding.Decode(decodedBinarySecretBytes, output.SecretBinary)
		if err != nil {
			return secrets.SecretVal{ErrorMsg: fmt.Sprintf("Can't base64 decode Secrets Manager secret: %v", err)}
		}
		return secrets.SecretVal{Value: string(decodedBinarySecretBytes[:secretLen])}
	}

	// should not happen but let's handle this gracefully
	return secrets.SecretVal{Value: "", ErrorMsg: "Secrets Manager returned something but there seems to be no data available"}
}
