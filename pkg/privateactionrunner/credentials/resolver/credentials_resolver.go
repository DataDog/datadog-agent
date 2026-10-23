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
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
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
	config                    model.Reader
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

// NewPrivateCredentialResolver validates configured credentials and retains the
// live config so subsequent tasks pick up secret refreshes. A nil config supports
// connections that do not reference runner credentials. Invalid Script credential
// file roots are skipped with a warning.
func NewPrivateCredentialResolver(config model.Reader, scriptCredentialFileAllowedRoots []string) (PrivateCredentialResolver, error) {
	if config != nil {
		if _, err := readCredentialValues(config); err != nil {
			return nil, err
		}
	}
	roots := make([]string, 0, len(scriptCredentialFileAllowedRoots))
	seen := make(map[string]struct{}, len(scriptCredentialFileAllowedRoots))
	for _, configuredRoot := range scriptCredentialFileAllowedRoots {
		path := filepath.Clean(configuredRoot)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		roots = append(roots, path)
		root, _, err := openScriptCredentialRoot(path)
		if err != nil {
			log.Warn("Script credential file root is currently unusable", log.String("root", configuredRoot), log.ErrorField(err))
			continue
		}
		_ = root.Close()
	}
	return &privateCredentialResolver{config: config, scriptCredentialFileRoots: roots}, nil
}

func readCredentialValues(config model.Reader) (map[string]par.CredentialConfig, error) {
	values := make(map[string]par.CredentialConfig)
	if err := structure.UnmarshalKey(config, par.CredentialsValues, &values, structure.ErrorUnused); err != nil {
		// Decoder errors can contain secret values; do not expose them to callers.
		return nil, fmt.Errorf("failed to decode %s", par.CredentialsValues)
	}
	return values, nil
}

func openScriptCredentialRoot(path string) (*os.Root, string, error) {
	if !filepath.IsAbs(path) {
		return nil, "", errScriptCredentialRootNotAbsolute
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(resolvedPath)
	if err != nil {
		return nil, "", err
	}
	openedInfo, openedErr := root.Stat(".")
	filesystemRootInfo, filesystemRootErr := os.Stat(filesystemRootPath(resolvedPath))
	if openedErr != nil || filesystemRootErr != nil || os.SameFile(openedInfo, filesystemRootInfo) {
		_ = root.Close()
		return nil, "", errCouldNotOpenScriptCredentialRoots
	}
	return root, resolvedPath, nil
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
	case privateactionspb.CredentialsType_CONNECTION_TOKENS_V2:
		resolvedTokens, err := p.resolveConnectionTokensV2(connInfo.GetTokensV2())
		if err != nil {
			return nil, err
		}
		tokens, details = privateconnection.ExtractConnectionDetails(&privateactionspb.ConnectionInfo{Tokens: resolvedTokens})
		credentialTokens, err := p.resolveTokenAuthTokens(ctx, tokens)
		if err != nil {
			return nil, err
		}
		return &privateconnection.PrivateCredentials{Tokens: credentialTokens, Type: privateconnection.TokenAuthType, HttpDetails: details}, nil
	}
	return nil, fmt.Errorf("unsupported credential type: %s", connInfo.CredentialsType)
}

func (p *privateCredentialResolver) resolveConnectionTokensV2(tokens []*privateactionspb.ConnectionTokenV2) ([]*privateactionspb.ConnectionToken, error) {
	resolved := make([]*privateactionspb.ConnectionToken, 0, len(tokens))
	var credentialValues map[string]par.CredentialConfig
	for _, token := range tokens {
		if token == nil || len(token.GetNameSegments()) == 0 {
			return nil, errors.New("connection token and its name must not be empty")
		}
		var value string
		switch source := token.GetSource().(type) {
		case *privateactionspb.ConnectionTokenV2_PlainText_:
			value = source.PlainText.GetValue()
		case *privateactionspb.ConnectionTokenV2_RunnerCredential_:
			if p.config == nil {
				return nil, errors.New("runner credential configuration is unavailable")
			}
			key := source.RunnerCredential.GetKey()
			if key == "" {
				return nil, fmt.Errorf("runner credential key for connection token %q must not be empty", connlib.GetName(token))
			}
			// Read once per task so all tokens use the same config snapshot.
			if credentialValues == nil {
				var err error
				credentialValues, err = readCredentialValues(p.config)
				if err != nil {
					return nil, err
				}
			}
			credential, found := credentialValues[key]
			if !found {
				return nil, fmt.Errorf("could not resolve connection token %q: requested runner credential is not available", connlib.GetName(token))
			}
			value = credential.Value
		default:
			return nil, fmt.Errorf("unsupported source for connection token %q", connlib.GetName(token))
		}
		resolved = append(resolved, privateconnection.NewPlainTextToken(token.GetNameSegments(), value))
	}
	return resolved, nil
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
	path, ok := cleanScriptCredentialPath(path)
	if !ok {
		return nil, errCouldNotLoadScriptCredentialFile
	}

	for _, rootPath := range p.scriptCredentialFileRoots {
		root, resolvedRootPath, err := openScriptCredentialRoot(rootPath)
		if err != nil {
			continue
		}
		file, err := openInScriptCredentialRoot(root, path, rootPath, resolvedRootPath)
		_ = root.Close()
		if err == nil {
			return file, nil
		}
	}

	return nil, errCouldNotLoadScriptCredentialFile
}

// The path may use either the configured or the resolved root form; os.Root enforces containment.
func openInScriptCredentialRoot(root *os.Root, path string, rootForms ...string) (credentialFile, error) {
	relativePath, ok := relativeToRoot(path, rootForms...)
	if !ok {
		return nil, errCouldNotLoadScriptCredentialFile
	}
	return root.Open(relativePath)
}

// ScriptCredentialFileAllowed reports whether the resolver would accept path under roots.
// A root that does not exist yet is matched by its configured path, since it may be created later.
func ScriptCredentialFileAllowed(roots []string, path string) bool {
	path, ok := cleanScriptCredentialPath(path)
	if !ok {
		return false
	}
	for _, configuredRoot := range roots {
		rootPath := filepath.Clean(configuredRoot)
		root, resolvedRootPath, err := openScriptCredentialRoot(rootPath)
		switch {
		case err == nil:
			_ = root.Close()
			if _, ok := relativeToRoot(path, rootPath, resolvedRootPath); ok {
				return true
			}
		case errors.Is(err, fs.ErrNotExist):
			if _, ok := relativeToRoot(path, rootPath); ok {
				return true
			}
		}
	}
	return false
}

// cleanScriptCredentialPath returns the cleaned path when it is absolute and has no ".." element.
func cleanScriptCredentialPath(path string) (string, bool) {
	if path == "" || !filepath.IsAbs(path) || containsParentPathElement(path) {
		return "", false
	}
	return filepath.Clean(path), true
}

// relativeToRoot returns path relative to the first root form that contains it.
func relativeToRoot(path string, rootForms ...string) (string, bool) {
	for _, rootForm := range rootForms {
		if relativePath, err := filepath.Rel(rootForm, path); err == nil && filepath.IsLocal(relativePath) {
			return relativePath, true
		}
	}
	return "", false
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
