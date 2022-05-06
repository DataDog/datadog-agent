// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package remote

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/meta"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/client/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf/data"
	"github.com/theupdateframework/go-tuf/pkg/keys"
	"github.com/theupdateframework/go-tuf/sign"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type testServer struct {
	pbgo.UnimplementedAgentSecureServer
	mock.Mock
}

func (s *testServer) ClientGetConfigs(ctx context.Context, req *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	args := s.Called(ctx, req)
	return args.Get(0).(*pbgo.ClientGetConfigsResponse), args.Error(1)
}

func getTestServer(t *testing.T) *testServer {
	hosts := []string{"127.0.0.1", "localhost", "::1"}
	_, rootCertPEM, rootKey, err := security.GenerateRootCert(hosts, 2048)
	require.NoError(t, err)
	rootKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey),
	})
	cert, err := tls.X509KeyPair(rootCertPEM, rootKeyPEM)
	if err != nil {
		panic(err)
	}

	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewServerTLSFromCert(&cert)),
	}
	server := grpc.NewServer(opts...)
	testServer := &testServer{}
	pbgo.RegisterAgentSecureServer(server, testServer)

	go func() {
		if err := server.Serve(listener); err != nil {
			panic(err)
		}
	}()
	dir, err := os.MkdirTemp("", "testserver")
	require.NoError(t, err)
	config.Datadog.Set("auth_token_file_path", dir+"/auth_token")
	config.Datadog.Set("cmd_port", listener.Addr().(*net.TCPAddr).Port)
	_, err = security.CreateOrFetchToken()
	require.NoError(t, err)
	return testServer
}

func TestClientEmptyResponse(t *testing.T) {
	testServer := getTestServer(t)

	embeddedRoot := generateRoot(generateKey(), 1, generateKey())
	config.Datadog.Set("remote_configuration.director_root", embeddedRoot)

	client, err := newClient("test-agent-name", []rdata.Product{rdata.ProductAPMSampling})
	assert.NoError(t, err)

	testServer.On("ClientGetConfigs", mock.Anything, &pbgo.ClientGetConfigsRequest{Client: &pbgo.Client{
		State: &pbgo.ClientState{
			RootVersion:    meta.RootsDirector().LastVersion(),
			TargetsVersion: 0,
			Error:          "",
		},
		Id:      client.stateClient.ID(),
		IsAgent: true,
		ClientAgent: &pbgo.ClientAgent{
			Name:    "test-agent-name",
			Version: version.AgentVersion,
		},
		Products: []string{string(rdata.ProductAPMSampling)},
	}}).Return(&pbgo.ClientGetConfigsResponse{
		Roots:       [][]byte{},
		Targets:     []byte{},
		TargetFiles: []*pbgo.File{},
	}, nil)

	err = client.poll()
	assert.NoError(t, err)
}

func TestClientValidResponse(t *testing.T) {
	testServer := getTestServer(t)

	targetsKey := generateKey()
	embeddedRoot := generateRoot(generateKey(), 1, targetsKey)
	apmConfig := apmsampling.APMSampling{
		TargetTPS: []apmsampling.TargetTPS{{Service: "service1", Env: "env1", Value: 4}},
	}
	rawApmConfig, err := apmConfig.MarshalMsg(nil)
	assert.NoError(t, err)
	target1 := generateTarget(rawApmConfig, 5)
	target2content, _ := generateRandomTarget(2)
	targets := generateTargets(targetsKey, 1, data.TargetFiles{"datadog/3/APM_SAMPLING/config-id-1/1": target1})
	config.Datadog.Set("remote_configuration.director_root", embeddedRoot)

	c, err := newClient("test-agent", []rdata.Product{rdata.ProductAPMSampling})
	assert.NoError(t, err)

	testServer.On("ClientGetConfigs", mock.Anything, &pbgo.ClientGetConfigsRequest{Client: &pbgo.Client{
		State: &pbgo.ClientState{
			RootVersion:    meta.RootsDirector().LastVersion(),
			TargetsVersion: 0,
			Error:          "",
		},
		Id:      c.stateClient.ID(),
		IsAgent: true,
		ClientAgent: &pbgo.ClientAgent{
			Name:    "test-agent",
			Version: version.AgentVersion,
		},
		Products: []string{string(rdata.ProductAPMSampling)},
	}}).Return(&pbgo.ClientGetConfigsResponse{
		Targets: targets,
		TargetFiles: []*pbgo.File{
			{Path: "datadog/3/APM_SAMPLING/config-id-1/1", Raw: rawApmConfig},
			{Path: "datadog/3/TESTING1/config-id-2/2", Raw: target2content},
		},
	}, nil)

	err = c.poll()
	assert.NoError(t, err)
	c.updateConfigs()
	apmUpdates := c.APMSamplingUpdates()
	require.Len(t, apmUpdates, 1)
	apmUpdate := <-apmUpdates
	assert.Len(t, apmUpdate, 1)
	assert.Equal(t, "config-id-1", apmUpdate[0].ID)
	assert.Equal(t, uint64(5), apmUpdate[0].Version)
	assert.Equal(t, apmConfig, apmUpdate[0].Config)
}

func generateKey() keys.Signer {
	key, _ := keys.GenerateEd25519Key()
	return key
}

func generateTargets(key keys.Signer, version int, targets data.TargetFiles) []byte {
	meta := data.NewTargets()
	meta.Expires = time.Now().Add(1 * time.Hour)
	meta.Version = version
	meta.Targets = targets
	signed, _ := sign.Marshal(&meta, key)
	serialized, _ := json.Marshal(signed)
	return serialized
}

func generateRoot(key keys.Signer, version int, targetsKey keys.Signer) []byte {
	root := data.NewRoot()
	root.Version = version
	root.Expires = time.Now().Add(1 * time.Hour)
	root.AddKey(key.PublicData())
	root.AddKey(targetsKey.PublicData())
	root.Roles["root"] = &data.Role{
		KeyIDs:    key.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["timestamp"] = &data.Role{
		KeyIDs:    key.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["targets"] = &data.Role{
		KeyIDs:    targetsKey.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["snapshot"] = &data.Role{
		KeyIDs:    key.PublicData().IDs(),
		Threshold: 1,
	}
	signedRoot, _ := sign.Marshal(&root, key)
	serializedRoot, _ := json.Marshal(signedRoot)
	return serializedRoot
}

func hashSha256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

func generateRandomTarget(version int) ([]byte, data.TargetFileMeta) {
	file := make([]byte, 128)
	rand.Read(file)
	return file, generateTarget(file, uint64(version))
}

type versionCustom struct {
	Version *uint64 `json:"v"`
}

func generateTarget(file []byte, version uint64) data.TargetFileMeta {
	custom, _ := json.Marshal(&versionCustom{Version: &version})
	customJSON := json.RawMessage(custom)
	return data.TargetFileMeta{
		FileMeta: data.FileMeta{
			Length: int64(len(file)),
			Hashes: data.Hashes{
				"sha256": hashSha256(file),
			},
			Custom: &customJSON,
		},
	}
}
