// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package resolver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func TestBasicAuthTokenFormats(t *testing.T) {
	resolver, err := NewPrivateCredentialResolver(nil)
	require.NoError(t, err)
	token := func(name, value string) *privateactionspb.ConnectionTokenV2 {
		return &privateactionspb.ConnectionTokenV2{
			NameSegments: []string{"root_tokens", name},
			Source:       &privateactionspb.ConnectionTokenV2_PlainText_{PlainText: &privateactionspb.ConnectionTokenV2_PlainText{Value: value}},
		}
	}
	username, password := token("username", "user"), token("password", "secret")
	path := filepath.Join(t.TempDir(), "basic.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"auth_type":"Basic Auth","credentials":[{"username":"user","password":"secret"}]}`), 0600))
	legacy := []*privateactionspb.ConnectionToken{
		privateconnection.NewPlainTextToken([]string{"root_tokens", "username"}, "user"),
		privateconnection.NewFileSecretToken([]string{"root_tokens", "password"}, path),
	}
	for _, tc := range []struct {
		name   string
		kind   privateactionspb.CredentialsType
		v2     []*privateactionspb.ConnectionTokenV2
		legacy []*privateactionspb.ConnectionToken
		want   privateconnection.AuthType
		err    string
	}{
		{name: "basic-v2", kind: privateactionspb.CredentialsType_BASIC_AUTH, v2: []*privateactionspb.ConnectionTokenV2{username, password}, want: privateconnection.BasicAuthType},
		{name: "basic-legacy", kind: privateactionspb.CredentialsType_BASIC_AUTH, legacy: legacy, want: privateconnection.BasicAuthType},
		{name: "generic-v2", kind: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2, v2: []*privateactionspb.ConnectionTokenV2{username, password}, want: privateconnection.TokenAuthType},
		{name: "missing-username", kind: privateactionspb.CredentialsType_BASIC_AUTH, v2: []*privateactionspb.ConnectionTokenV2{password}, err: "requires username and password"},
		{name: "missing-password", kind: privateactionspb.CredentialsType_BASIC_AUTH, v2: []*privateactionspb.ConnectionTokenV2{username}, err: "requires username and password"},
		{name: "empty", kind: privateactionspb.CredentialsType_BASIC_AUTH, err: "no credential found"},
		{name: "mixed-basic", kind: privateactionspb.CredentialsType_BASIC_AUTH, v2: []*privateactionspb.ConnectionTokenV2{username, password}, legacy: legacy, err: "must not mix"},
		{name: "mixed-generic", kind: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2, v2: []*privateactionspb.ConnectionTokenV2{username, password}, legacy: legacy, err: "must not mix"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := &privateactionspb.ConnectionInfo{CredentialsType: tc.kind, Tokens: tc.legacy, TokensV2: tc.v2}
			wire, err := proto.Marshal(conn)
			require.NoError(t, err)
			decoded := &privateactionspb.ConnectionInfo{}
			require.NoError(t, proto.Unmarshal(wire, decoded))
			got, err := resolver.ResolveConnectionInfoToCredential(context.Background(), decoded, nil)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got.Type)
			require.Equal(t, "user", got.AsTokenMap()["username"])
			require.Equal(t, "secret", got.AsTokenMap()["password"])
		})
	}
}
