// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package enrollment

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"gopkg.in/yaml.v3"
)

const (
	// Constants from reference implementation
	TokenHashHmacKey      = "enrollment-token-fingerprint-v1"
	BindingKeyHmacKey     = "enrollment-token-binding-key-v1"
	AccountBindingJwtType = "account_registration_request+jwt"
)

var AllowedSitesToRegion = map[string]string{
	"datadoghq.com":     "us1",
	"us3.datadoghq.com": "us3",
	"us5.datadoghq.com": "us5",
	"datadoghq.eu":      "eu1",
	"ap1.datadoghq.com": "ap1",
	"ap2.datadoghq.com": "ap2",
}

// RunnerConfig represents the YAML configuration structure
type RunnerConfig struct {
	RunnerId         string              `yaml:"runnerId"`
	OrgId            int64               `yaml:"orgId"`
	PrivateKey       string              `yaml:"privateKey"`
	Modes            []string            `yaml:"modes"`
	ActionsAllowlist map[string][]string `yaml:"actionsAllowlist"`
	Allowlist        []string            `yaml:"allowlist"`
	AllowIMDS        bool                `yaml:"allowImds"`
}

// ProvisionRunnerIdentityWithToken performs enrollment with a provided token and outputs to stdout
func ProvisionRunnerIdentityWithToken(enrollmentToken, datadogSite, appendToFile string) error {
	fmt.Println("Starting runner enrollment...")
	return runEnrollmentToConfig(enrollmentToken, datadogSite, appendToFile)
}

// ProvisionRunnerIdentityWithAPIKey performs self-enrollment using API key authentication
func ProvisionRunnerIdentityWithAPIKey(apiKey, appKey, datadogSite string, selfAuth bool, appendToFile string) error {
	fmt.Println("Starting runner self-enrollment...")
	return runSelfEnrollmentToConfig(apiKey, appKey, datadogSite, selfAuth, appendToFile)
}

// generateKeys creates a new ECDSA P-256 key pair
func generateKeys() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// buildHmacKey creates HMAC key using the pattern from reference implementation
func buildHmacKey(key, payload string) []byte {
	hmacHash := hmac.New(sha256.New, []byte(key))
	hmacHash.Write([]byte(payload))
	return hmacHash.Sum(nil)
}

// runEnrollmentToConfig performs enrollment and outputs configuration to stdout
func runEnrollmentToConfig(enrollmentToken, datadogSite, appendToFile string) error {
	// Generate ECDSA key pair
	privateKey, err := generateKeys()
	if err != nil {
		return fmt.Errorf("failed to generate keys: %w", err)
	}

	// Convert private key to JWK
	privateKeyJWK, err := utils.EcdsaToJWK(privateKey)
	if err != nil {
		return fmt.Errorf("failed to convert private key to JWK: %w", err)
	}

	// Convert public key to JWK
	publicKeyJWK, err := utils.EcdsaToJWK(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to convert public key to JWK: %w", err)
	}

	// Build enrollment URL
	enrollmentUrl := fmt.Sprintf("https://api.%s/api/v2/on-prem-management-service/enrollments/complete", datadogSite)

	// Create token hash and binding key following reference implementation
	tokenHash := base64.RawURLEncoding.EncodeToString(buildHmacKey(TokenHashHmacKey, enrollmentToken))
	bindingKey := buildHmacKey(BindingKeyHmacKey, enrollmentToken)

	// Create binding signer (first JWT)
	bindingSigner, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.HS256,
		Key:       bindingKey,
	}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			"kid":           tokenHash,
			jose.HeaderType: "account_binding+jwt",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create binding signer: %w", err)
	}

	// Create binding JWT with public key as claims
	binding, err := jwt.Signed(bindingSigner).Claims(*publicKeyJWK).Serialize()
	if err != nil {
		return fmt.Errorf("failed to sign account binding: %w", err)
	}

	// Create account binding signer (outer JWT)
	accountBindingSigner, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.ES256,
		Key:       privateKeyJWK,
	}, &jose.SignerOptions{
		EmbedJWK: true,
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			jose.HeaderType: AccountBindingJwtType,
			"url":           enrollmentUrl,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create account binding signer: %w", err)
	}

	// Create the final signed JWT with the binding as a claim
	signedAccountBinding, err := jwt.Signed(accountBindingSigner).Claims(map[string]any{
		"externalAccountBinding": binding,
	}).Serialize()
	if err != nil {
		return fmt.Errorf("failed to sign account binding: %w", err)
	}

	// Send enrollment request using OPMS client with JWT body
	ddHost := strings.Join([]string{"api", datadogSite}, ".")
	enrollmentClient := opms.NewEnrollmentClient(ddHost)
	response, err := enrollmentClient.SendEnrollmentJWT(context.Background(), signedAccountBinding)
	if err != nil {
		return fmt.Errorf("enrollment request failed: %w", err)
	}

	// Get region for URN construction
	region, ok := AllowedSitesToRegion[datadogSite]
	if !ok {
		region = "us1" // Default to us1 if site is not recognized
		log.Infof("Unrecognized site '%s', defaulting to region 'us1'", datadogSite)
	}

	err = outputConfig(response, privateKeyJWK, region, appendToFile)
	if err != nil {
		return fmt.Errorf("failed to generate configuration: %w", err)
	}

	return nil
}

func getEc2Identity() (*opms.Ec2Identity, error) {
	ctx := context.Background()
	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	if awsConfig.Region == "" {
		log.Warn("AWS region not found in config, defaulting to us-east-1")
		awsConfig.Region = "us-east-1"
	}

	stsClient := sts.NewFromConfig(awsConfig)
	presigner := sts.NewPresignClient(stsClient)
	identity, err := presigner.PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to presign GetCallerIdentity: %w", err)
	}

	parsedURL, err := url.Parse(identity.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse caller identity URL: %w", err)
	}

	return &opms.Ec2Identity{
		Region:         awsConfig.Region,
		Authentication: parsedURL.RawQuery,
	}, nil
}

// runSelfEnrollmentToConfig performs self-enrollment with API key and outputs configuration to stdout
func runSelfEnrollmentToConfig(apiKey, appKey, datadogSite string, selfAuth bool, appendToFile string) error {
	// Get EC2 identity with region and PKCS7
	var ec2Identity *opms.Ec2Identity

	if selfAuth {
		var err error
		ec2Identity, err = getEc2Identity()
		if err != nil {
			return fmt.Errorf("failed to get EC2 identity: %w", err)
		}
	}

	// Generate ECDSA key pair
	privateKey, err := generateKeys()
	if err != nil {
		return fmt.Errorf("failed to generate keys: %w", err)
	}

	// Convert private key to JWK
	privateKeyJWK, err := utils.EcdsaToJWK(privateKey)
	if err != nil {
		return fmt.Errorf("failed to convert private key to JWK: %w", err)
	}

	// Convert public key to JWK
	publicKeyJWK, err := utils.EcdsaToJWK(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to convert public key to JWK: %w", err)
	}

	// Marshal public key to JSON for sending to the API
	publicKeyJSON, err := publicKeyJWK.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal public key to JSON: %w", err)
	}

	// Send self-enrollment request using OPMS client with API key
	ddHost := strings.Join([]string{"api", datadogSite}, ".")
	enrollmentClient := opms.NewEnrollmentClient(ddHost)
	response, err := enrollmentClient.SendSelfEnrollmentRequest(context.Background(), apiKey, appKey, string(publicKeyJSON), ec2Identity)
	if err != nil {
		return fmt.Errorf("self-enrollment request failed: %w", err)
	}

	// Get region for URN construction
	region, ok := AllowedSitesToRegion[datadogSite]
	if !ok {
		region = "us1" // Default to us1 if site is not recognized
		log.Infof("Unrecognized site '%s', defaulting to region 'us1'", datadogSite)
	}

	err = outputConfig(response, privateKeyJWK, region, appendToFile)
	if err != nil {
		return fmt.Errorf("failed to generate configuration: %w", err)
	}

	return nil
}

func outputConfig(response *opms.EnrollmentResponse, jwk *jose.JSONWebKey, region string, appendToFile string) error {
	marshalledPrivateJwk, err := jwk.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}
	urn := fmt.Sprintf("urn:dd:apps:on-prem-runner:%s:%d:%s", region, response.OrgId, response.RunnerId)

	config := map[string]interface{}{
		"privateactionrunner": map[string]interface{}{
			"enabled":           true,
			"private_key":       base64.RawURLEncoding.EncodeToString(marshalledPrivateJwk),
			"urn":               urn,
			"modes":             response.Modes,
			"actions_allowlist": response.ActionsAllowlist,
		},
	}

	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	// Output to stdout
	fmt.Printf("Enrollment successful! Runner ID: %s\n", response.RunnerId)
	fmt.Printf("Org ID: %d\n", response.OrgId)
	fmt.Printf("URN: %s\n", urn)
	fmt.Printf("Modes: %s\n", strings.Join(response.Modes, ", "))
	fmt.Printf("\nAdd the following to your datadog.yaml:\n\n%s", string(yamlData))

	if appendToFile != "" {
		f, err := os.OpenFile(appendToFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer f.Close()

		_, err = f.WriteString("\n" + string(yamlData))
		if err != nil {
			return fmt.Errorf("failed to append config to file: %w", err)
		}

		fmt.Printf("\nConfiguration appended to %s\n", appendToFile)
	}

	return nil
}
