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
	"path/filepath"

	"github.com/google/uuid"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	connlib "github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/connection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

const (
	maxCredentialsFileSize = 1 * 1024 * 1024 // 1 MB
)

var (
	errCouldNotLoadCredentialFile        = errors.New("could not load credentials file")
	errCouldNotParseCredentialFile       = errors.New("could not parse credentials file")
	errCouldNotLoadScriptCredentialFile  = errors.New("could not load script credential file")
	errCouldNotOpenScriptCredentialRoots = errors.New("could not open script credential file roots")
	errScriptCredentialRootNotAbsolute   = errors.New("script credential file root must be absolute")
)

type PrivateCredentialResolver interface {
	ResolveConnectionInfoToCredential(ctx context.Context, conn *privateactionspb.ConnectionInfo, userUUID *uuid.UUID) (*privateconnection.PrivateCredentials, error)
}
type privateCredentialResolver struct {
	scriptCredentialFileRoots []string
}

type credentialFile interface {
	io.Reader
	io.Closer
	Stat() (os.FileInfo, error)
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

func NewPrivateCredentialResolver(scriptCredentialFileAllowedRoots []string) PrivateCredentialResolver {
	roots := make([]string, 0, len(scriptCredentialFileAllowedRoots))
	seen := make(map[string]struct{}, len(scriptCredentialFileAllowedRoots))
	for _, configuredRoot := range scriptCredentialFileAllowedRoots {
		path := filepath.Clean(configuredRoot)
		if _, ok := seen[path]; ok {
			continue
		}
		root, err := openScriptCredentialRoot(path)
		if err != nil {
			log.Warn("Skipping Script credential file root", log.String("root", configuredRoot), log.ErrorField(err))
			continue
		}
		_ = root.Close()
		seen[path] = struct{}{}
		roots = append(roots, path)
	}
	return &privateCredentialResolver{scriptCredentialFileRoots: roots}
}

func openScriptCredentialRoot(path string) (*os.Root, error) {
	if !filepath.IsAbs(path) {
		return nil, errScriptCredentialRootNotAbsolute
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	openedInfo, openedErr := root.Stat(".")
	resolvedInfo, resolvedErr := os.Stat(resolvedPath)
	filesystemRootInfo, filesystemRootErr := os.Stat(filesystemRootPath(resolvedPath))
	if openedErr != nil || resolvedErr != nil || filesystemRootErr != nil || !os.SameFile(openedInfo, resolvedInfo) || os.SameFile(openedInfo, filesystemRootInfo) {
		_ = root.Close()
		return nil, errCouldNotOpenScriptCredentialRoots
	}
	return root, nil
}

func filesystemRootPath(path string) string {
	return filepath.VolumeName(path) + string(filepath.Separator)
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

func (p *privateCredentialResolver) resolveTokenAuthTokens(ctx context.Context, tokens []*privateactionspb.ConnectionToken) ([]privateconnection.PrivateCredentialsToken, error) {
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
			resolved, err := p.resolveYamlFileToken(ctx, t.YamlFile.GetPath())
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

func (p *privateCredentialResolver) resolveYamlFileToken(ctx context.Context, path string) (string, error) {
	data, err := readCredentialFile(ctx, func() (credentialFile, error) {
		return p.openScriptCredentialFile(path)
	}, errCouldNotLoadScriptCredentialFile)
	if err != nil {
		return "", err
	}
	// TODO: this should probably also do the yaml parsing and validation but for now we're using runtimepb.Credential_TokenCredential_Token which only supports strings
	// so its the responsibility of the action to desarialize the yaml and validate it
	return string(data), nil
}

func (p *privateCredentialResolver) openScriptCredentialFile(path string) (credentialFile, error) {
	if path == "" || !filepath.IsAbs(path) || containsParentPathElement(path) {
		return nil, errCouldNotLoadScriptCredentialFile
	}
	path = filepath.Clean(path)

	for _, rootPath := range p.scriptCredentialFileRoots {
		relativePath, err := filepath.Rel(rootPath, path)
		if err != nil || !filepath.IsLocal(relativePath) {
			continue
		}

		root, err := openScriptCredentialRoot(rootPath)
		if err != nil {
			continue
		}
		file, err := root.Open(relativePath)
		_ = root.Close()
		if err != nil {
			continue
		}
		return file, nil
	}

	return nil, errCouldNotLoadScriptCredentialFile
}

func containsParentPathElement(path string) bool {
	elementStart := 0
	for i := 0; i < len(path); i++ {
		if os.IsPathSeparator(path[i]) {
			if path[elementStart:i] == ".." {
				return true
			}
			elementStart = i + 1
		}
	}
	return path[elementStart:] == ".."
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
		return "", errors.New("credential file contains unsupported authentication type")
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

	data, err := readCredentialFile(ctx, func() (credentialFile, error) {
		return os.Open(path)
	}, errCouldNotLoadCredentialFile)
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(data, config)
	if err != nil {
		return config, errCouldNotParseCredentialFile
	}
	return config, nil
}

func readCredentialFile(ctx context.Context, open func() (credentialFile, error), publicErr error) ([]byte, error) {
	file, err := open()
	if err != nil {
		return nil, publicErr
	}
	defer closeSafely(ctx, file)

	stat, err := file.Stat()
	if err != nil {
		return nil, publicErr
	}
	if !stat.Mode().IsRegular() || stat.Size() == 0 || stat.Size() > maxCredentialsFileSize {
		return nil, publicErr
	}

	data, err := io.ReadAll(io.LimitReader(file, maxCredentialsFileSize+1))
	if err != nil {
		return nil, publicErr
	}
	if len(data) == 0 || len(data) > maxCredentialsFileSize {
		return nil, publicErr
	}

	return data, nil
}

func closeSafely(ctx context.Context, closer io.Closer) {
	err := closer.Close()
	if err != nil {
		log.FromContext(ctx).Warn("failed to close credentials file safely")
	}
}
