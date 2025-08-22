// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package credentials

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/uuid"
)

const (
	maxCredentialsFileSize = 1 * 1024 * 1024 // 1 MB
)

type PrivateCredentialResolver interface {
	ResolveConnectionInfoToCredential(ctx context.Context, conn *privateactions.ConnectionInfo, userUUID *uuid.UUID) (interface{}, error)
}

type privateCredentialResolver struct {
}

func NewPrivateCredentialResolver() PrivateCredentialResolver {
	return &privateCredentialResolver{}
}

func (p privateCredentialResolver) ResolveConnectionInfoToCredential(ctx context.Context, conn *privateactions.ConnectionInfo, userUUID *uuid.UUID) (interface{}, error) {
	switch conn.CredentialsType {
	case privateactions.CredentialsType_TOKEN_AUTH:
		return resolveTokenAuthTokens(ctx, conn.Tokens)
	}
	log.Errorf("Unsupported credentials type: %s, skipping resolution", conn.CredentialsType)
	return nil, nil
}

func resolveTokenAuthTokens(ctx context.Context, tokens []*privateactions.ConnectionToken) (interface{}, error) {
	credentialTokens := make(map[string]interface{})
	for _, token := range tokens {
		tokenName := getTokenName(token)
		switch t := token.GetTokenValue().(type) {
		case *privateactions.ConnectionToken_YamlFile_:
			resolved, err := resolveYamlFileToken(ctx, t.YamlFile.GetPath())
			if err != nil {
				return nil, err
			}
			credentialTokens[tokenName] = resolved
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
	// TODO: this should probably also do the yaml parsing and validation but keeping it the same as the reference implementation for now
	// so its the responsibility of the action to desarialize the yaml and validate it
	return string(data), nil
}

func validateAndReadFile(ctx context.Context, path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("credential file path is empty")
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
		return nil, fmt.Errorf("the credentials file is empty")
	}
	if stat.Size() > maxCredentialsFileSize {
		return nil, fmt.Errorf("the credentials file is too large")
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
		log.Warn("failed to close credentials file safely")
	}
}

func getTokenName(token *privateactions.ConnectionToken) string {
	return token.GetNameSegments()[len(token.GetNameSegments())-1]
}
