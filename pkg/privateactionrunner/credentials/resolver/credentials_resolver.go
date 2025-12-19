// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	connlib "github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/connection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

const (
	maxCredentialsFileSize = 1 * 1024 * 1024 // 1 MB
)

type PrivateCredentialResolver interface {
	ResolveConnectionInfoToCredential(ctx context.Context, conn *privateactionspb.ConnectionInfo, userUUID *uuid.UUID) (*privateconnection.PrivateCredentials, error)
}
type privateCredentialResolver struct {
}

type PrivateConnectionConfig struct {
	AuthType    privateconnection.AuthType `json:"auth_type"`
	Credentials []Credential               `json:"credentials"`
}

type Credential struct {
	TokenName  string `json:"tokenName,omitempty"`
	TokenValue string `json:"tokenValue,omitempty"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
}

func NewPrivateCredentialResolver() PrivateCredentialResolver {
	return &privateCredentialResolver{}
}

func (p *privateCredentialResolver) ResolveConnectionInfoToCredential(ctx context.Context, connInfo *privateactionspb.ConnectionInfo, userUUID *uuid.UUID) (*privateconnection.PrivateCredentials, error) {
	tokens, details := privateconnection.ExtractConnectionDetails(connInfo)
	switch connInfo.CredentialsType {
	case privateactionspb.CredentialsType_TOKEN_AUTH:
		credentialTokens, err := resolveTokenAuthTokens(ctx, tokens)
		if err != nil {
			return nil, err
		}
		return &privateconnection.PrivateCredentials{
			Tokens:      credentialTokens,
			Type:        privateconnection.TokenAuthType,
			HttpDetails: details,
		}, nil
	case privateactionspb.CredentialsType_BASIC_AUTH:
		credentialTokens, err := resolveBasicAuthTokens(ctx, tokens)
		if err != nil {
			return nil, err
		}
		return &privateconnection.PrivateCredentials{
			Tokens:      credentialTokens,
			Type:        privateconnection.BasicAuthType,
			HttpDetails: details,
		}, nil
	}
	return nil, fmt.Errorf("unsupported credential type: %s", connInfo.CredentialsType)
}

func resolveTokenAuthTokens(ctx context.Context, tokens []*privateactionspb.ConnectionToken) ([]privateconnection.PrivateCredentialsToken, error) {
	credentialTokens := make([]privateconnection.PrivateCredentialsToken, 0)
	for _, token := range tokens {
		tokenName := connlib.GetName(token)
		switch t := token.GetTokenValue().(type) {
		case *privateactionspb.ConnectionToken_PlainText_:
			credentialTokens = append(credentialTokens, privateconnection.PrivateCredentialsToken{
				Name: tokenName, Value: t.PlainText.GetValue(),
			})
		case *privateactionspb.ConnectionToken_FileSecret_:
			secret, err := getSecretFromDockerLocation(ctx, t.FileSecret.GetPath(), tokenName)
			if err != nil {
				return nil, err
			}
			credentialTokens = append(credentialTokens, privateconnection.PrivateCredentialsToken{
				Name: tokenName, Value: secret,
			})
		case *privateactionspb.ConnectionToken_YamlFile_:
			resolved, err := resolveYamlFileToken(ctx, t.YamlFile.GetPath())
			if err != nil {
				return nil, err
			}
			credentialTokens = append(credentialTokens, privateconnection.PrivateCredentialsToken{
				Name: tokenName, Value: resolved,
			})
		default:
			return nil, fmt.Errorf("unsupported on prem token kind: %T", token.GetTokenValue())
		}
	}
	return credentialTokens, nil
}

func resolveYamlFileToken(ctx context.Context, path string) (string, error) {
	data, err := validateAndReadFile(ctx, path)
	if err != nil {
		return "", err
	}
	// TODO: this should probably also do the yaml parsing and validation but for now we're using runtimepb.Credential_TokenCredential_Token which only supports strings
	// so its the responsibility of the action to desarialize the yaml and validate it
	return string(data), nil
}

func resolveBasicAuthTokens(ctx context.Context, tokens []*privateactionspb.ConnectionToken) ([]privateconnection.PrivateCredentialsToken, error) {
	var username string
	var pwdToken *privateactionspb.ConnectionToken
	for _, token := range tokens {
		tokenName := connlib.GetName(token)
		switch tokenName {
		case privateconnection.UsernameTokenName:
			username = token.GetPlainText().GetValue()
		case privateconnection.PasswordTokenName:
			pwdToken = token
		}
	}
	if pwdToken == nil || username == "" {
		return []privateconnection.PrivateCredentialsToken{}, errors.New("no credential found")
	}
	secret, err := getSecretFromDockerLocation(ctx, pwdToken.GetFileSecret().GetPath(), username)
	if err != nil {
		return []privateconnection.PrivateCredentialsToken{}, err
	}
	return []privateconnection.PrivateCredentialsToken{
		{
			Name:  privateconnection.UsernameTokenName,
			Value: username,
		},
		{
			Name:  privateconnection.PasswordTokenName,
			Value: secret,
		},
	}, nil
}

func getSecretFromDockerLocation(
	ctx context.Context,
	dockerSecretPath string,
	secretName string,
) (secret string, err error) {
	privateCredentialConfig, err := loadConnectionCredentials(ctx, dockerSecretPath)
	if err != nil {
		return "", err
	}
	switch privateCredentialConfig.AuthType {
	case privateconnection.TokenAuthType:
		for _, cred := range privateCredentialConfig.Credentials {
			if cred.TokenName == secretName {
				return cred.TokenValue, nil
			}
		}
	case privateconnection.BasicAuthType:
		for _, cred := range privateCredentialConfig.Credentials {
			if cred.Username == secretName {
				return cred.Password, nil
			}
		}
	default:
		return "", fmt.Errorf("the credential provided in the config file is not supported: invalid auth_type \"%s\"", privateCredentialConfig.AuthType)
	}
	log.FromContext(ctx).Warn("credential not found in file", log.String("path", dockerSecretPath), log.String("secretName", secretName))
	return "", nil
}

/**
 * Connection config file format:
 * For token auth:
 * {
 *   "auth_type": "Token Auth",
 *   "credentials": [
 *     {
 *       "tokenName": "your-token-name-1",
 *       "tokenValue": "your-token-value-1"
 *     },
 *     {
 *       "tokenName": "your-token-name-2",
 *       "tokenValue": "your-token-value-2"
 *     }
 *   ]
 * }
 *
 * For Basic Auth:
 * {
 *   "auth_type": "Basic Auth",
 *   "credentials": [
 *     {
 *       "username": "your-username",
 *       "password": "your-password"
 *     }
 *   ]
 * }
 */
func loadConnectionCredentials(ctx context.Context, path string) (config *PrivateConnectionConfig, err error) {
	config = &PrivateConnectionConfig{}

	data, err := validateAndReadFile(ctx, path)
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("could not unmarshal credentials file: %v", err)
	}
	return config, nil
}

func validateAndReadFile(ctx context.Context, path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("credential file path is empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open credentials file: %v ", err)
	}
	defer closeSafely(ctx, file)

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if stat.Size() == 0 {
		return nil, errors.New("the credentials file is empty")
	}
	if stat.Size() > maxCredentialsFileSize {
		return nil, errors.New("the credentials file is too large")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not load credentials file: %v", err)
	}

	return data, nil
}

func closeSafely(ctx context.Context, closer io.Closer) {
	err := closer.Close()
	if err != nil {
		log.FromContext(ctx).Warn("failed to close credentials file safely")
	}
}
