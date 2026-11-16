// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/go-tuf/data"
	"github.com/DataDog/go-tuf/pkg/keys"
	"github.com/DataDog/go-tuf/sign"
	"github.com/DataDog/go-tuf/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// dcRepository models one TUF director repository: a root file signed by a set
// of root keys, and targets signed by the repository's targets key.
type dcRepository struct {
	rootKey     keys.Signer
	targetsKey  keys.Signer
	signedRoot  []byte
	signedRoots [][]byte // signed roots to deliver to a fresh-join client, from the embedded version up
	targets     []byte
	targetPath  string
	targetFile  []byte
}

func generateKey() keys.Signer {
	key, err := keys.GenerateEd25519Key()
	if err != nil {
		panic(err)
	}
	return key
}

// buildDCRoot signs a director root of the given version with key, rotating
// the targets role to targetsKey. When previousRootKey is not nil the root is
// also signed by it, modeling a DC rotation from the shared embedded root: a
// client latched on the previous root accepts the new one.
func buildDCRoot(key keys.Signer, targetsKey keys.Signer, previousRootKey keys.Signer, version int64) []byte {
	root := data.NewRoot()
	root.Version = version
	root.Expires = time.Now().Add(24 * time.Hour)
	root.AddKey(key.PublicData())
	root.AddKey(targetsKey.PublicData())
	rootRole := &data.Role{
		KeyIDs:    key.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["root"] = rootRole
	targetsRole := &data.Role{
		KeyIDs:    targetsKey.PublicData().IDs(),
		Threshold: 1,
	}
	root.Roles["targets"] = targetsRole
	root.Roles["timestamp"] = rootRole
	root.Roles["snapshot"] = rootRole

	signers := []keys.Signer{key}
	if previousRootKey != nil {
		signers = append(signers, previousRootKey)
	}
	signedRoot, err := sign.Marshal(&root, signers...)
	if err != nil {
		panic(err)
	}
	signedRootBytes, err := json.Marshal(&signedRoot)
	if err != nil {
		panic(err)
	}
	return signedRootBytes
}

func buildDCTargets(targetsKey keys.Signer, path string, file []byte, version int64) []byte {
	targets := data.NewTargets()
	targets.Version = version

	targetFileMeta, err := util.GenerateTargetFileMeta(bytes.NewBuffer(file), []string{"sha256", "sha512"}...)
	if err != nil {
		panic(err)
	}
	custom := json.RawMessage(fmt.Sprintf(`{"v": %d}`, version))
	targetFileMeta.Custom = &custom
	targets.Targets[path] = targetFileMeta

	signedTargets, err := sign.Marshal(targets, targetsKey)
	if err != nil {
		panic(err)
	}
	targetsBytes, err := json.Marshal(signedTargets)
	if err != nil {
		panic(err)
	}
	return targetsBytes
}

// newDCRepositories builds the DC switch topology: a shared embedded-style
// director root of the given version, and two DCs that each rotated once to
// their own keys, cross-signing the embedded root's key so a fresh join
// accepts their chain.
func newDCRepositories(embeddedRootVersion int64, targetPath string) (embeddedRoot []byte, dcA *dcRepository, dcB *dcRepository) {
	embeddedRootKey := generateKey()
	embeddedTargetsKey := generateKey()
	embeddedRoot = buildDCRoot(embeddedRootKey, embeddedTargetsKey, nil, embeddedRootVersion)

	newDC := func(targetFile []byte) *dcRepository {
		rootKey := generateKey()
		targetsKey := generateKey()
		dc := &dcRepository{
			rootKey:    rootKey,
			targetsKey: targetsKey,
		}
		dc.signedRoot = buildDCRoot(rootKey, targetsKey, embeddedRootKey, embeddedRootVersion+1)
		dc.signedRoots = [][]byte{dc.signedRoot}
		dc.targetPath = targetPath
		dc.targetFile = targetFile
		dc.targets = buildDCTargets(targetsKey, targetPath, targetFile, 1)
		return dc
	}

	dcA = newDC([]byte("config from dc A"))
	dcB = newDC([]byte("config from dc B"))

	return embeddedRoot, dcA, dcB
}

// dcResponse builds a ClientGetConfigsResponse serving one DC repository. The
// roots are only included when freshJoin is true: the core agent serves roots
// strictly newer than the version reported by the client, so a client latched
// at the same root version as the new DC (the DC switch case) receives none.
func dcResponse(dc *dcRepository, freshJoin bool) *pbgo.ClientGetConfigsResponse {
	response := &pbgo.ClientGetConfigsResponse{
		Targets:       dc.targets,
		TargetFiles:   []*pbgo.File{{Path: dc.targetPath, Raw: dc.targetFile}},
		ClientConfigs: []string{dc.targetPath},
	}
	if freshJoin {
		response.Roots = dc.signedRoots
	}
	return response
}

// fakeConfigFetcher records the requests sent by the client and answers them
// with the provided function.
type fakeConfigFetcher struct {
	requests []*pbgo.ClientGetConfigsRequest
	answer   func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error)
}

func (f *fakeConfigFetcher) ClientGetConfigs(_ context.Context, request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	f.requests = append(f.requests, request)
	return f.answer(request)
}

func newDCClient(t *testing.T, embeddedRoot []byte) (*Client, *fakeConfigFetcher) {
	t.Helper()

	fetcher := &fakeConfigFetcher{}
	client, err := newClient(fetcher, WithDirectorRootOverride("datadoghq.com", string(embeddedRoot)), WithProducts(state.ProductCWSDD))
	require.NoError(t, err)
	return client, fetcher
}

func lastRequest(t *testing.T, fetcher *fakeConfigFetcher) *pbgo.ClientGetConfigsRequest {
	t.Helper()
	require.NotEmpty(t, fetcher.requests)
	return fetcher.requests[len(fetcher.requests)-1]
}

func TestUpdateResetsTUFStateOnVerificationError(t *testing.T) {
	embeddedRoot, dcA, dcB := newDCRepositories(17, "datadog/2/CWS_DD/id1/config")
	client, fetcher := newDCClient(t, embeddedRoot)

	// The client first joins through DC A: it latches A's root version.
	fetcher.answer = func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return dcResponse(dcA, request.Client.State.RootVersion == 17), nil
	}
	require.NoError(t, client.update())
	configs := client.GetConfigs(state.ProductCWSDD)
	require.Len(t, configs, 1)
	assert.Equal(t, dcA.targetFile, configs[dcA.targetPath].Config)

	// The org moves to DC B: the update carries B's targets but no root file
	// (both DCs sit at root version 18), so it cannot be applied against A's
	// latched state. This is what triggers the reset.
	latchedRootVersion := uint64(0)
	fetcher.answer = func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		latchedRootVersion = request.Client.State.RootVersion
		return dcResponse(dcB, false), nil
	}
	require.Error(t, client.update())
	assert.EqualValues(t, 18, latchedRootVersion)

	// The failed update dropped the TUF state: the next request is a fresh
	// join reporting the embedded root version, and B's full chain verifies
	// from there.
	fetcher.answer = func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return dcResponse(dcB, request.Client.State.RootVersion == 17), nil
	}
	require.NoError(t, client.update())
	freshJoin := lastRequest(t, fetcher)
	assert.EqualValues(t, 17, freshJoin.Client.State.RootVersion)
	assert.Empty(t, freshJoin.Client.State.ConfigStates)
	assert.Empty(t, freshJoin.CachedTargetFiles)

	configs = client.GetConfigs(state.ProductCWSDD)
	require.Len(t, configs, 1)
	assert.Equal(t, dcB.targetFile, configs[dcB.targetPath].Config)

	// The Director proof was dropped with the state: it only carries the new
	// DC's chain, not stale roots from DC A (both chains sit at version 18
	// and cannot be merged).
	proof, ok := client.GetConfigTUFProof(dcB.targetPath)
	require.True(t, ok)
	require.Len(t, proof.Roots, 1)
	assert.Equal(t, dcB.signedRoot, proof.Roots[0])
	assert.Equal(t, dcB.targets, proof.Targets)
	assert.Equal(t, dcB.targetFile, proof.TargetFile)
}

func TestUpdateTUFStateResetCooldown(t *testing.T) {
	embeddedRoot, dcA, dcB := newDCRepositories(17, "datadog/2/CWS_DD/id1/config")
	client, fetcher := newDCClient(t, embeddedRoot)

	now := time.Now()
	client.now = func() time.Time { return now }

	// The client latches DC B.
	fetcher.answer = func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return dcResponse(dcB, request.Client.State.RootVersion == 17), nil
	}
	require.NoError(t, client.update())

	// A failing update drops the TUF state (first reset), so the client
	// re-joins and latches whichever chain the core agent serves, here the
	// old DC again (the core agent has not recovered yet).
	fetcher.answer = func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return dcResponse(dcA, request.Client.State.RootVersion == 17), nil
	}
	require.Error(t, client.update())
	require.NoError(t, client.update())
	assert.EqualValues(t, 17, lastRequest(t, fetcher).Client.State.RootVersion)

	// A failing update within the cooldown does not drop the state again:
	// the client keeps reporting the latched version instead of re-joining
	// on every poll.
	now = now.Add(time.Minute)
	fetcher.answer = func(*pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return dcResponse(dcB, false), nil
	}
	require.Error(t, client.update())
	require.Error(t, client.update())
	assert.EqualValues(t, 18, lastRequest(t, fetcher).Client.State.RootVersion)

	// After the cooldown, a failing update can drop the state again.
	now = now.Add(tufStateResetCooldown)
	fetcher.answer = func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return dcResponse(dcB, request.Client.State.RootVersion == 17), nil
	}
	require.Error(t, client.update())
	require.NoError(t, client.update())
	assert.EqualValues(t, 17, lastRequest(t, fetcher).Client.State.RootVersion)
}

// dcEmptyConfigResponse builds a response for a DC that serves valid TUF
// metadata but no configurations at all, modeling a new DC without configs
// for the client's products.
func dcEmptyConfigResponse(dc *dcRepository, freshJoin bool) *pbgo.ClientGetConfigsResponse {
	targets := data.NewTargets()
	targets.Version = 1
	signedTargets, err := sign.Marshal(targets, dc.targetsKey)
	if err != nil {
		panic(err)
	}
	targetsBytes, err := json.Marshal(signedTargets)
	if err != nil {
		panic(err)
	}
	response := &pbgo.ClientGetConfigsResponse{Targets: targetsBytes}
	if freshJoin {
		response.Roots = dc.signedRoots
	}
	return response
}

func TestUpdateNotifiesListenersOfConfigsRemovedByReset(t *testing.T) {
	embeddedRoot, dcA, dcB := newDCRepositories(17, "datadog/2/CWS_DD/id1/config")
	client, fetcher := newDCClient(t, embeddedRoot)

	var updates []map[string]state.RawConfig
	client.Subscribe(state.ProductCWSDD, func(update map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
		updates = append(updates, update)
	})

	// The client joins through DC A and receives one config.
	fetcher.answer = func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return dcResponse(dcA, request.Client.State.RootVersion == 17), nil
	}
	require.NoError(t, client.update())
	require.Len(t, updates, 1)
	require.Len(t, updates[0], 1)

	// The org moves to DC B, which serves no configs for this product. The
	// first update fails (no roots against A's latched state) and triggers
	// the reset; the fresh-join update then succeeds with an empty config
	// list.
	fetcher.answer = func(*pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return dcEmptyConfigResponse(dcB, false), nil
	}
	require.Error(t, client.update())

	// The fresh repository cannot detect the config's removal by diffing
	// (it starts empty), so the listener must be notified explicitly with
	// an empty config map.
	fetcher.answer = func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return dcEmptyConfigResponse(dcB, request.Client.State.RootVersion == 17), nil
	}
	require.NoError(t, client.update())
	require.Len(t, updates, 2)
	require.NotNil(t, updates[1])
	assert.Empty(t, updates[1])

	// The pending removals are consumed: a subsequent successful update
	// does not notify the listener again.
	require.NoError(t, client.update())
	require.Len(t, updates, 2)
}

func TestUpdateApplyStatusConcurrentWithReset(t *testing.T) {
	embeddedRoot, dcA, dcB := newDCRepositories(17, "datadog/2/CWS_DD/id1/config")
	client, fetcher := newDCClient(t, embeddedRoot)

	fetcher.answer = func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return dcResponse(dcA, request.Client.State.RootVersion == 17), nil
	}
	require.NoError(t, client.update())

	// Hammer UpdateApplyStatus while the poll goroutine resets the TUF
	// state: the c.state pointer must be read under the client lock.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			client.UpdateApplyStatus(dcA.targetPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	}()

	// A DC switch fails the update and drops the state; the fresh join then
	// re-latches on the old DC. Run several reset cycles, bypassing the
	// cooldown, to give the race detector a wide window.
	for i := 1; i <= 3; i++ {
		cooldownEnd := time.Now().Add(time.Duration(i) * tufStateResetCooldown)
		client.now = func() time.Time { return cooldownEnd }
		fetcher.answer = func(*pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
			return dcResponse(dcB, false), nil
		}
		require.Error(t, client.update())
		fetcher.answer = func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
			return dcResponse(dcA, request.Client.State.RootVersion == 17), nil
		}
		require.NoError(t, client.update())
	}

	close(stop)
	wg.Wait()
}

func TestUpdateTransportErrorDoesNotResetTUFState(t *testing.T) {
	embeddedRoot, dcA, _ := newDCRepositories(17, "datadog/2/CWS_DD/id1/config")
	client, fetcher := newDCClient(t, embeddedRoot)

	fetcher.answer = func(request *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return dcResponse(dcA, request.Client.State.RootVersion == 17), nil
	}
	require.NoError(t, client.update())

	// A transport error (the core agent being unreachable) leaves the
	// latched state alone: only updates that cannot be applied, meaning the
	// TUF metadata did not verify, trigger the reset.
	fetcher.answer = func(*pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
		return nil, status.Error(codes.Unavailable, "agent is down")
	}
	require.Error(t, client.update())
	require.Error(t, client.update())
	assert.EqualValues(t, 18, lastRequest(t, fetcher).Client.State.RootVersion)
}
