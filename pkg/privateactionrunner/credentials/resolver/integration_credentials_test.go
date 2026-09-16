// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package resolver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

type staticIntegrationConfigProvider struct {
	configs []integration.Config
}

func (p staticIntegrationConfigProvider) GetIntegrationConfigs(context.Context) ([]integration.Config, error) {
	return p.configs, nil
}

func integrationConnection(name string, selectors map[string]string) *privateactionspb.ConnectionInfo {
	return &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{{
			Source: &privateactionspb.ConnectionTokenV2_IntegrationCredential{
				IntegrationCredential: &privateactionspb.IntegrationCredential{
					Integration: name,
					Selectors:   selectors,
				},
			},
		}},
	}
}

func TestResolveIntegrationCredentials(t *testing.T) {
	provider := staticIntegrationConfigProvider{configs: []integration.Config{
		{
			Name: "postgres",
			Instances: []integration.Data{
				[]byte("host: first.example.com\nport: 5432\nusername: first\npassword: first-secret\n"),
				[]byte("host: selected.example.com\nusername: selected\npassword: selected-secret\ndbname: inventory\n"),
			},
		},
	}}
	credentialResolver := NewPrivateCredentialResolver(nil, provider)
	conn := integrationConnection("postgres", map[string]string{"hostname": "selected.example.com"})

	credentials, err := credentialResolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"host":     "selected.example.com",
		"username": "selected",
		"password": "selected-secret",
		"dbname":   "inventory",
		"database": "inventory",
		"port":     "5432",
	}, credentials.AsTokenMap())
}

func TestResolveIntegrationCredentialsSupportsNestedSelectors(t *testing.T) {
	provider := staticIntegrationConfigProvider{configs: []integration.Config{{
		Name:      "custom",
		Instances: []integration.Data{[]byte("host: selected.example.com\noptions:\n  role: primary\npassword: secret\n")},
	}}}
	credentialResolver := NewPrivateCredentialResolver(nil, provider)
	conn := integrationConnection("custom", map[string]string{"options.role": "primary"})

	credentials, err := credentialResolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	require.NoError(t, err)
	assert.Equal(t, "secret", credentials.AsTokenMap()["password"])
}

func TestResolveIntegrationCredentialsRequiresUniqueMatch(t *testing.T) {
	provider := staticIntegrationConfigProvider{configs: []integration.Config{{
		Name: "mysql",
		Instances: []integration.Data{
			[]byte("host: duplicated.example.com\nusername: first\n"),
			[]byte("host: duplicated.example.com\nusername: second\n"),
		},
	}}}
	credentialResolver := NewPrivateCredentialResolver(nil, provider)
	conn := integrationConnection("mysql", map[string]string{"host": "duplicated.example.com"})

	_, err := credentialResolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	assert.EqualError(t, err, "integration credential selector matched more than one instance")
}

func TestResolveIntegrationCredentialsDisabled(t *testing.T) {
	credentialResolver := NewPrivateCredentialResolver(nil, nil)
	conn := integrationConnection("postgres", nil)

	_, err := credentialResolver.ResolveConnectionInfoToCredential(context.Background(), conn, nil)
	assert.EqualError(t, err, "integration credentials are disabled")
}

func TestCredentialsFromMongoServerURI(t *testing.T) {
	credentials := credentialsFromIntegrationInstance("mongo", map[string]any{
		"server": "mongodb://db-user:db-password@mongo.example.com/admin?authSource=users&tls=false",
	})

	assert.Equal(t, map[string]string{
		"server":     "mongodb://db-user:db-password@mongo.example.com/admin?authSource=users&tls=false",
		"host":       "mongo.example.com",
		"port":       "27017",
		"username":   "db-user",
		"password":   "db-password",
		"database":   "admin",
		"authSource": "users",
		"tls":        "false",
	}, credentials)
}
