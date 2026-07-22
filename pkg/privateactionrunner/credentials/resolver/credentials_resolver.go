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
	"strings"

	"github.com/google/uuid"
	yaml "gopkg.in/yaml.v3"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	connlib "github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/connection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

const (
	maxCredentialsFileSize = 1 * 1024 * 1024 // 1 MB
	// secretOrigin identifies the private action runner as the source of the
	// configuration passed to the secret backend (used in `agent secret` output).
	secretOrigin = "private-action-runner"
)

type PrivateCredentialResolver interface {
	ResolveConnectionInfoToCredential(ctx context.Context, conn *privateactionspb.ConnectionInfo, userUUID *uuid.UUID) (*privateconnection.PrivateCredentials, error)
}
type privateCredentialResolver struct {
	// secretResolver resolves ENC[...] handles through the Datadog secret backend.
	// It may be nil when no secrets component is available, in which case handles
	// are left untouched.
	secretResolver secrets.Component
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

func NewPrivateCredentialResolver(secretResolver secrets.Component) PrivateCredentialResolver {
	return &privateCredentialResolver{secretResolver: secretResolver}
}

func (p *privateCredentialResolver) ResolveConnectionInfoToCredential(ctx context.Context, connInfo *privateactionspb.ConnectionInfo, userUUID *uuid.UUID) (*privateconnection.PrivateCredentials, error) {
	if connInfo == nil {
		return nil, nil
	}
	tokens, details := privateconnection.ExtractConnectionDetails(connInfo)
	switch connInfo.CredentialsType {
	case privateactionspb.CredentialsType_TOKEN_AUTH:
		credentialTokens, err := p.resolveTokenAuthTokens(ctx, tokens)
		if err != nil {
			return nil, err
		}
		log.Infof("Received credentials tokens: %v", credentialTokens)
		return &privateconnection.PrivateCredentials{
			Tokens:      credentialTokens,
			Type:        privateconnection.TokenAuthType,
			HttpDetails: details,
		}, nil
	case privateactionspb.CredentialsType_BASIC_AUTH:
		credentialTokens, err := p.resolveBasicAuthTokens(ctx, tokens)
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

func (p *privateCredentialResolver) resolveTokenAuthTokens(ctx context.Context, tokens []*privateactionspb.ConnectionToken) ([]privateconnection.PrivateCredentialsToken, error) {
	credentialTokens := make([]privateconnection.PrivateCredentialsToken, 0)
	for _, token := range tokens {
		tokenName := connlib.GetName(token)
		switch t := token.GetTokenValue().(type) {
		case *privateactionspb.ConnectionToken_PlainText_:
			value, err := p.resolveSecret(ctx, t.PlainText.GetValue())
			if err != nil {
				return nil, err
			}
			credentialTokens = append(credentialTokens, privateconnection.PrivateCredentialsToken{
				Name: tokenName, Value: value,
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

func (p *privateCredentialResolver) resolveBasicAuthTokens(ctx context.Context, tokens []*privateactionspb.ConnectionToken) ([]privateconnection.PrivateCredentialsToken, error) {
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

	username, err := p.resolveSecret(ctx, username)
	if err != nil {
		return []privateconnection.PrivateCredentialsToken{}, err
	}

	var secret string
	switch pwd := pwdToken.GetTokenValue().(type) {
	case *privateactionspb.ConnectionToken_PlainText_:
		// The password is provided inline; it may itself be an ENC[...] secret handle.
		secret, err = p.resolveSecret(ctx, pwd.PlainText.GetValue())
	default:
		secret, err = getSecretFromDockerLocation(ctx, pwdToken.GetFileSecret().GetPath(), username)
	}
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

// resolveSecret resolves an ENC[...] secret handle through the configured Datadog
// secret backend. Values that are not secret handles are returned unchanged, as are
// handles when no secret backend is available.
//
// It supports the structured handle format documented at
// https://docs.datadoghq.com/agent/configuration/secrets-management/#id-for-secrets
// (e.g. "ENC[aws_secrets;My-Secrets;password]"), which is understood by the secret
// backend itself.
func (p *privateCredentialResolver) resolveSecret(ctx context.Context, value string) (string, error) {
	if p.secretResolver == nil || !isEncrypted(value) {
		return value, nil
	}

	// Wrap the handle in a minimal YAML document so the secret backend can walk it
	// and substitute the resolved value. YAML marshalling keeps the handle intact
	// regardless of the characters it contains.
	doc, err := yaml.Marshal(map[string]string{"value": value})
	if err != nil {
		return "", fmt.Errorf("could not marshal secret handle: %w", err)
	}
	resolved, err := p.secretResolver.Resolve(doc, secretOrigin, "", "", false)
	if err != nil {
		return "", fmt.Errorf("could not resolve secret handle: %w", err)
	}
	var out struct {
		Value string `yaml:"value"`
	}
	if err := yaml.Unmarshal(resolved, &out); err != nil {
		return "", fmt.Errorf("could not unmarshal resolved secret: %w", err)
	}
	return out.Value, nil
}

// isEncrypted reports whether s is a Datadog secret backend handle in the ENC[...] format.
func isEncrypted(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "ENC[") && strings.HasSuffix(s, "]")
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
