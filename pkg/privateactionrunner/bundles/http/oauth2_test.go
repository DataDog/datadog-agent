// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/httpclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials/resolver"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

// tlsTestProvider trusts the test identity provider's certificate, since the
// token URL must be https.
type tlsTestProvider struct{ transport http.RoundTripper }

func (p tlsTestProvider) NewDefaultClient() (httpclient.HTTPClient, error) {
	return &http.Client{Transport: p.transport}, nil
}

func (p tlsTestProvider) NewClient(c *httpclient.RunnerHttpClientConfig) (httpclient.HTTPClient, error) {
	client := &http.Client{Transport: p.transport}
	if c.DisableRedirects {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return client, nil
}

type fakeIdentityProvider struct {
	*httptest.Server
	requests atomic.Int32
	lastForm url.Values
	lastUser string
	lastPass string
}

func newFakeIdentityProvider(t *testing.T) *fakeIdentityProvider {
	idp := &fakeIdentityProvider{}
	idp.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := idp.requests.Add(1)
		require.NoError(t, r.ParseForm())
		idp.lastForm = r.PostForm
		idp.lastUser, idp.lastPass, _ = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"at-%d","token_type":"Bearer","expires_in":3600}`, n)
	}))
	t.Cleanup(idp.Close)
	return idp
}

func oauth2Connection(baseURL, tokenURL string) *privateconnection.PrivateCredentials {
	return &privateconnection.PrivateCredentials{
		Type: privateconnection.TokenAuthType,
		HttpDetails: privateconnection.HttpDetails{
			BaseURL: baseURL,
			OAuth2: &privateconnection.OAuth2Details{
				GrantType:    privateconnection.OAuth2ClientCredentialsGrant,
				TokenURL:     tokenURL,
				ClientID:     "par-client",
				ClientSecret: "s3cret&x",
				Scopes:       []string{"api.read", "api.write"},
			},
		},
	}
}

func newOAuth2Handler(idp *fakeIdentityProvider) *Handler {
	return NewHttpRequestAction(&config.Config{HTTPTimeout: 5 * time.Second}).
		WithHttpClientProvider(tlsTestProvider{transport: idp.Client().Transport})
}

func requestTask(inputs map[string]interface{}) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{BundleID: "com.datadoghq.http", Name: "request", Inputs: inputs, SecDatadogHeaderValue: "sec-value"}
	return task
}

// The runner gets a client-credentials token with the connection's secret,
// sends it to the connection's host, and reuses it until it expires.
func TestOAuth2ClientCredentialsAddsAndCachesToken(t *testing.T) {
	idp := newFakeIdentityProvider(t)
	var authorizations []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer api.Close()
	handler := newOAuth2Handler(idp)
	credential := oauth2Connection(api.URL, idp.URL+"/oauth2/token")

	for range 2 {
		output, err := handler.Run(context.Background(), requestTask(map[string]interface{}{"verb": "GET", "url": api.URL + "/items"}), credential)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, output.(*RequestOutputs).Status)
	}
	assert.Equal(t, []string{"Bearer at-1", "Bearer at-1"}, authorizations)
	assert.EqualValues(t, 1, idp.requests.Load())
	assert.Equal(t, "client_credentials", idp.lastForm.Get("grant_type"))
	assert.Equal(t, "api.read api.write", idp.lastForm.Get("scope"))
	assert.Empty(t, idp.lastForm.Get("client_secret"))
	// RFC 6749 section 2.3.1: both parts are form-encoded inside Basic.
	assert.Equal(t, "par-client", idp.lastUser)
	assert.Equal(t, "s3cret%26x", idp.lastPass)

	// Expired tokens are replaced.
	handler.oauth2.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	_, err := handler.Run(context.Background(), requestTask(map[string]interface{}{"verb": "GET", "url": api.URL + "/items"}), credential)
	require.NoError(t, err)
	assert.Equal(t, "Bearer at-2", authorizations[2])
}

// A 401 means the identity provider revoked the token early, so the next task
// gets a new one instead of failing until the cached one expires.
func TestOAuth2UnauthorizedDropsCachedToken(t *testing.T) {
	idp := newFakeIdentityProvider(t)
	var authorizations []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		if len(authorizations) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer api.Close()
	handler := newOAuth2Handler(idp)
	credential := oauth2Connection(api.URL, idp.URL+"/token")
	task := requestTask(map[string]interface{}{"verb": "GET", "url": api.URL})

	_, err := handler.Run(context.Background(), task, credential)
	require.Error(t, err)
	_, err = handler.Run(context.Background(), task, credential)
	require.NoError(t, err)
	assert.Equal(t, []string{"Bearer at-1", "Bearer at-2"}, authorizations)
}

func TestOAuth2ClientSecretPost(t *testing.T) {
	idp := newFakeIdentityProvider(t)
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer api.Close()
	credential := oauth2Connection(api.URL, idp.URL+"/token")
	credential.HttpDetails.OAuth2.ClientAuthMethod = privateconnection.OAuth2ClientSecretPost

	_, err := newOAuth2Handler(idp).Run(context.Background(), requestTask(map[string]interface{}{"verb": "GET", "url": api.URL}), credential)
	require.NoError(t, err)
	assert.Equal(t, "par-client", idp.lastForm.Get("client_id"))
	assert.Equal(t, "s3cret&x", idp.lastForm.Get("client_secret"))
	assert.Empty(t, idp.lastUser)
}

// In raw mode the token reaches the connection's host only, like other
// connection headers, so a call to another host never carries it.
func TestOAuth2TokenScopedToConnectionHostInRawMode(t *testing.T) {
	idp := newFakeIdentityProvider(t)
	var apiAuthorization, otherAuthorization string
	api := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { apiAuthorization = r.Header.Get("Authorization") }))
	defer api.Close()
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { otherAuthorization = r.Header.Get("Authorization") }))
	defer other.Close()
	handler := newOAuth2Handler(idp)
	credential := oauth2Connection(api.URL, idp.URL+"/token")

	for _, target := range []string{api.URL, other.URL} {
		_, err := handler.Run(context.Background(), requestTask(map[string]interface{}{"responseMode": ResponseModeRaw, "verb": "POST", "url": target + "/mcp"}), credential)
		require.NoError(t, err)
	}
	assert.Equal(t, "Bearer at-1", apiAuthorization)
	assert.Empty(t, otherAuthorization)
}

func TestOAuth2RejectsCallerAuthorization(t *testing.T) {
	idp := newFakeIdentityProvider(t)
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("no request expected") }))
	defer api.Close()

	_, err := newOAuth2Handler(idp).Run(context.Background(), requestTask(map[string]interface{}{
		"verb": "GET", "url": api.URL,
		"requestHeaders": []interface{}{map[string]interface{}{"key": "authorization", "value": []string{"Bearer other"}}},
	}), oauth2Connection(api.URL, idp.URL+"/token"))
	require.ErrorContains(t, err, "set Authorization")
	assert.Zero(t, idp.requests.Load())
}

func TestOAuth2InvalidConnections(t *testing.T) {
	idp := newFakeIdentityProvider(t)
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("no request expected") }))
	defer api.Close()
	for name, mutate := range map[string]func(*privateconnection.PrivateCredentials){
		"no base URL": func(c *privateconnection.PrivateCredentials) { c.HttpDetails.BaseURL = "" },
		"http token URL": func(c *privateconnection.PrivateCredentials) {
			c.HttpDetails.OAuth2.TokenURL = "http://idp.internal/token"
		},
		"grant type":       func(c *privateconnection.PrivateCredentials) { c.HttpDetails.OAuth2.GrantType = "password" },
		"no client secret": func(c *privateconnection.PrivateCredentials) { c.HttpDetails.OAuth2.ClientSecret = "" },
		"client auth method": func(c *privateconnection.PrivateCredentials) {
			c.HttpDetails.OAuth2.ClientAuthMethod = "private_key_jwt"
		},
	} {
		t.Run(name, func(t *testing.T) {
			credential := oauth2Connection(api.URL, idp.URL+"/token")
			mutate(credential)
			_, err := newOAuth2Handler(idp).Run(context.Background(), requestTask(map[string]interface{}{"verb": "GET", "url": api.URL}), credential)
			require.Error(t, err)
		})
	}
	assert.Zero(t, idp.requests.Load())
}

func TestOAuth2TokenErrorDoesNotLeakSecret(t *testing.T) {
	idp := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad secret s3cret&x"}`))
	}))
	defer idp.Close()
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("no request expected") }))
	defer api.Close()
	handler := NewHttpRequestAction(&config.Config{HTTPTimeout: 5 * time.Second}).WithHttpClientProvider(tlsTestProvider{transport: idp.Client().Transport})

	_, err := handler.Run(context.Background(), requestTask(map[string]interface{}{"verb": "GET", "url": api.URL}), oauth2Connection(api.URL, idp.URL+"/token"))
	require.ErrorContains(t, err, "invalid_client")
	assert.NotContains(t, err.Error(), "s3cret")
}

// End to end from the connection: the client secret comes from a runner
// credential, and its allowed_http_domains apply to the token URL, where the
// secret is sent, not to the request URL.
func TestOAuth2ClientSecretFromRunnerCredential(t *testing.T) {
	for _, test := range []struct {
		name      string
		domains   string
		wantError string
	}{
		{name: "token URL allowed", domains: "[127.0.0.1]"},
		{name: "token URL denied", domains: "[idp.example.com]", wantError: "OAuth2 token URL is not allowed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			idp := newFakeIdentityProvider(t)
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer at-1" {
					w.WriteHeader(http.StatusUnauthorized)
				}
			}))
			defer api.Close()
			catalog := configmock.NewFromYAML(t, fmt.Sprintf(`
private_action_runner:
  credentials:
    values:
      idp_client_secret:
        value: s3cret&x
        allowed_actions: [com.datadoghq.http.request]
        allowed_http_domains: %s
`, test.domains))
			credentialResolver, err := resolver.NewPrivateCredentialResolver(catalog)
			require.NoError(t, err)
			plain := func(segments []string, value string) *privateactionspb.ConnectionTokenV2 {
				return &privateactionspb.ConnectionTokenV2{NameSegments: segments, Source: &privateactionspb.ConnectionTokenV2_PlainText_{
					PlainText: &privateactionspb.ConnectionTokenV2_PlainText{Value: value},
				}}
			}
			task := requestTask(map[string]interface{}{"verb": "GET", "url": api.URL + "/items"})
			task.Data.Attributes.ConnectionInfo = &privateactionspb.ConnectionInfo{
				CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
				TokensV2: []*privateactionspb.ConnectionTokenV2{
					plain([]string{"base_url"}, api.URL),
					plain([]string{"oauth2", "grant_type"}, "client_credentials"),
					plain([]string{"oauth2", "token_url"}, idp.URL+"/token"),
					plain([]string{"oauth2", "client_id"}, "par-client"),
					plain([]string{"oauth2", "scopes"}, "api.read api.write"),
					{NameSegments: []string{"oauth2", "client_secret"}, Source: &privateactionspb.ConnectionTokenV2_RunnerCredential_{
						RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: "idp_client_secret"},
					}},
				},
			}

			credentials, err := credentialResolver.ResolveCredentials(context.Background(), task)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			output, err := newOAuth2Handler(idp).Run(context.Background(), task, credentials)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, output.(*RequestOutputs).Status)
			assert.Equal(t, "s3cret%26x", idp.lastPass)
			assert.Equal(t, "api.read api.write", idp.lastForm.Get("scope"))
		})
	}
}
