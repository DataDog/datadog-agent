// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package resolver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	runnerconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	httpbundle "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/http"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func TestResolveBasicAuthV2FromRunnerCredentials(t *testing.T) {
	config := configmock.New(t)
	config.Set(par.CredentialsValues, map[string]any{
		"http_user":     map[string]any{"value": "runner-user"},
		"http_password": map[string]any{"value": "runner-password"},
	}, model.SourceFile)
	resolver, err := NewPrivateCredentialResolver(config)
	require.NoError(t, err)
	conn := &privateactionspb.ConnectionInfo{CredentialsType: privateactionspb.CredentialsType_BASIC_AUTH}
	for _, entry := range []struct{ name, key string }{{"username", "http_user"}, {"password", "http_password"}} {
		conn.TokensV2 = append(conn.TokensV2, &privateactionspb.ConnectionTokenV2{
			NameSegments: []string{"root_tokens", entry.name},
			Source:       &privateactionspb.ConnectionTokenV2_RunnerCredential_{RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: entry.key}},
		})
	}
	wire, err := proto.Marshal(conn)
	require.NoError(t, err)
	decoded := &privateactionspb.ConnectionInfo{}
	require.NoError(t, proto.Unmarshal(wire, decoded))
	got, err := resolver.ResolveConnectionInfoToCredential(context.Background(), decoded, nil)
	require.NoError(t, err)
	require.Equal(t, privateconnection.BasicAuthType, got.Type)
	username, password, err := got.GetUsernamePasswordBasicAuth()
	require.NoError(t, err)
	assert.Equal(t, "runner-user", username)
	assert.Equal(t, "runner-password", password)
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{Inputs: map[string]interface{}{"verb": "GET", "url": server.URL}}
	action := httpbundle.NewHttpRequestAction(&runnerconfig.Config{HTTPTimeout: 5 * time.Second})
	output, err := action.Run(context.Background(), task, got)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, output.(*httpbundle.RequestOutputs).Status)
	require.Equal(t, "Basic cnVubmVyLXVzZXI6cnVubmVyLXBhc3N3b3Jk", <-received)

	// Generic V2 username/password tokens must not accidentally become Basic auth.
	decoded.CredentialsType = privateactionspb.CredentialsType_CONNECTION_TOKENS_V2
	got, err = resolver.ResolveConnectionInfoToCredential(context.Background(), decoded, nil)
	require.NoError(t, err)
	assert.Equal(t, privateconnection.TokenAuthType, got.Type)
	decoded.CredentialsType = privateactionspb.CredentialsType(999)
	got, err = resolver.ResolveConnectionInfoToCredential(context.Background(), decoded, nil)
	require.ErrorContains(t, err, "unsupported credential type")
	assert.Nil(t, got)
}
