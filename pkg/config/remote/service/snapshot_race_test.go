// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/go-tuf/data"

	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// fakeRaceAPI is a hand-written api.API fake (rather than a testify mock)
// for the concurrency test below: it needs to return a new, valid,
// non-repeating response on every call from many concurrent goroutines,
// which doesn't fit testify's per-call `.On` expectation model well.
type fakeRaceAPI struct {
	counter atomic.Int64
}

func (f *fakeRaceAPI) Fetch(_ context.Context, _ *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	n := f.counter.Add(1)
	path := fmt.Sprintf("datadog/2/AGENT_CONFIG/gen-%d/config.json", n)
	return &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{
			Path: path,
			Raw:  []byte(fmt.Sprintf("content-for-generation-%d", n)),
		}},
	}, nil
}

func (f *fakeRaceAPI) FetchOrgData(context.Context) (*pbgo.OrgDataResponse, error) {
	return &pbgo.OrgDataResponse{Uuid: "race-test-org"}, nil
}

func (f *fakeRaceAPI) FetchOrgStatus(context.Context) (*pbgo.OrgStatusResponse, error) {
	return &pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil
}

func (f *fakeRaceAPI) UpdatePARJWT(string) {}
func (f *fakeRaceAPI) UpdateAPIKey(string) {}

// fakeRaceUptane is a hand-written coreAgentUptaneClient fake standing in for
// go-tuf: it doesn't do any real TUF verification, but it does maintain
// internally-consistent state across Update() calls (a strictly increasing
// "generation" reflected in TUFVersionState/Targets/TargetFiles/etc.) so the
// test below can assert that a single ClientGetConfigs response is never a
// mix of two generations.
//
// It's guarded by its own mutex to accurately model go-tuf's real
// constraint -- only the single refresh-loop writer (CoreAgentService.mu)
// ever calls into it, so in a correct implementation this lock is never
// contended. If ClientGetConfigs is ever changed to call into the uptane
// client again, concurrent reads/writes here would surface that as a data
// race under `-race`, which is exactly what this test is trying to catch.
type fakeRaceUptane struct {
	mu         sync.Mutex
	generation uint64
}

func (f *fakeRaceUptane) Update(_ *pbgo.LatestConfigsResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.generation++
	return nil
}

func (f *fakeRaceUptane) currentGeneration() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.generation
}

func (f *fakeRaceUptane) path(gen uint64) string {
	return fmt.Sprintf("datadog/2/AGENT_CONFIG/gen-%d/config.json", gen)
}

func (f *fakeRaceUptane) TUFVersionState() (uptane.TUFVersions, error) {
	return uptane.TUFVersions{DirectorRoot: 1, DirectorTargets: f.currentGeneration()}, nil
}

func (f *fakeRaceUptane) DirectorRoot(version uint64) ([]byte, error) {
	return []byte(fmt.Sprintf("root-%d", version)), nil
}

func (f *fakeRaceUptane) StoredOrgUUID() (string, error) { return "race-test-org", nil }

func (f *fakeRaceUptane) Targets() (data.TargetFiles, error) {
	gen := f.currentGeneration()
	if gen == 0 {
		return data.TargetFiles{}, nil
	}
	return data.TargetFiles{
		f.path(gen): {FileMeta: data.FileMeta{Length: int64(len(fmt.Sprintf("content-for-generation-%d", gen)))}},
	}, nil
}

func (f *fakeRaceUptane) TargetFile(path string) ([]byte, error) {
	files, err := f.TargetFiles([]string{path})
	if err != nil {
		return nil, err
	}
	return files[path], nil
}

func (f *fakeRaceUptane) TargetFiles(paths []string) (map[string][]byte, error) {
	gen := f.currentGeneration()
	out := make(map[string][]byte, len(paths))
	for _, p := range paths {
		if p == f.path(gen) {
			out[p] = []byte(fmt.Sprintf("content-for-generation-%d", gen))
		}
	}
	return out, nil
}

func (f *fakeRaceUptane) TargetsMeta() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"generation":%d}`, f.currentGeneration())), nil
}

func (f *fakeRaceUptane) UnsafeTargetsMeta() ([]byte, error) {
	return f.TargetsMeta()
}

func (f *fakeRaceUptane) TargetsCustom() ([]byte, error) { return []byte(`{}`), nil }

func (f *fakeRaceUptane) TimestampExpires() (time.Time, error) {
	return time.Now().Add(time.Hour), nil
}

func (f *fakeRaceUptane) Close() error { return nil }

func (f *fakeRaceUptane) GetTransactionalStoreMetadata() (*uptane.Metadata, error) {
	return &uptane.Metadata{}, nil
}

func (f *fakeRaceUptane) State() (uptane.State, error) { return uptane.State{}, nil }

// TestSnapshotConsistencyUnderConcurrentRefreshAndReads is the primary
// correctness test for the lock-free read path: many goroutines continuously
// call ClientGetConfigs while another goroutine continuously calls refresh(),
// for a fixed duration, run under `-race` (via `dda inv test
// --targets=./pkg/config/remote/service --race`).
//
// It asserts two things beyond "no panic, no data race" (which -race itself
// catches):
//
//  1. Every non-empty response returned during the run is internally
//     consistent: the TUF metadata (Targets bytes) and the target file
//     content it returns always agree on which refresh generation they came
//     from. Each fake generation N's target file path/content are unique to
//     that generation (see fakeRaceUptane), so if ClientGetConfigs ever
//     mixed fields from two different snapshot generations within one
//     response, decoding the generation out of the Targets metadata and out
//     of the returned file path/content would disagree.
//  2. The snapshot's generation counter (readSnapshot.generation, exposed
//     here only for this assertion) is monotonically non-decreasing across
//     every observation by every goroutine -- i.e. no goroutine ever
//     observes an older snapshot after having already observed a newer one,
//     which would indicate the lock-free swap is not actually publishing
//     atomically/safely.
func TestSnapshotConsistencyUnderConcurrentRefreshAndReads(t *testing.T) {
	const (
		numReaders = 16
		duration   = 500 * time.Millisecond
	)

	api := &fakeRaceAPI{}
	uptaneClient := &fakeRaceUptane{}
	realClock := clock.New()
	service := newTestService(t, api, uptaneClient, realClock)

	// Give every simulated client a distinct ID so ClientGetConfigs treats
	// them as already-active (avoiding the new-client bypass path, which
	// would otherwise serialize every reader on the same triggered refresh).
	seenClients := make([]*pbgo.Client, numReaders)
	for i := range seenClients {
		seenClients[i] = &pbgo.Client{
			Id:          fmt.Sprintf("race-test-client-%d", i),
			IsAgent:     true,
			ClientAgent: &pbgo.ClientAgent{},
			Products:    []string{"AGENT_CONFIG"},
			State:       &pbgo.ClientState{RootVersion: 1},
		}
		service.clients.seen(seenClients[i])
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// lastSeenGeneration is only used to assert monotonicity within a single
	// goroutine's own observations, so a plain slice indexed by reader ID
	// (each written by exactly one goroutine) is fine -- no shared mutable
	// state between goroutines here.
	var responses atomic.Int64
	var mismatches atomic.Int64

	// Single writer: repeatedly calls refresh(), exactly like the poll loop.
	// Uses assert (not require) since testify's require.FailNow must only be
	// called from the goroutine running the test function itself.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			assert.NoError(t, service.refresh())
		}
	}()

	// Many concurrent readers. All assertions here use assert (not require),
	// since testify's require.FailNow must only be called from the goroutine
	// running the test function itself.
	for i := 0; i < numReaders; i++ {
		client := seenClients[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			var lastGeneration int64
			for {
				select {
				case <-stop:
					return
				default:
				}

				snapBefore := service.snapshot.Load()
				resp, err := service.ClientGetConfigs(context.Background(), &pbgo.ClientGetConfigsRequest{
					Client: client,
				})
				if !assert.NoError(t, err) {
					continue
				}

				// Monotonicity: no reader should ever observe a snapshot
				// older than one it already observed. We can't read exactly
				// which generation served this particular response from the
				// response alone in the "no update" case, so we bound it
				// using the snapshot generation available immediately
				// before the call -- that's a lower bound on what could have
				// been served (a concurrent refresh may have published a
				// newer one in between, which is fine).
				if snapBefore.generation < lastGeneration {
					mismatches.Add(1)
					t.Errorf("observed snapshot generation %d after already having observed %d", snapBefore.generation, lastGeneration)
				}
				lastGeneration = snapBefore.generation

				if len(resp.TargetFiles) == 0 {
					continue
				}
				responses.Add(1)

				// Consistency: decode the generation from the TUF metadata
				// and from the returned file's path/content independently,
				// and assert they agree -- if ClientGetConfigs ever mixed
				// two generations within one response, this would catch it.
				var metaGen int
				_, err = fmt.Sscanf(string(resp.Targets), `{"generation":%d}`, &metaGen)
				if !assert.NoError(t, err, "unparseable Targets metadata: %s", resp.Targets) {
					continue
				}

				if !assert.Len(t, resp.TargetFiles, 1) {
					continue
				}
				file := resp.TargetFiles[0]
				var pathGen int
				_, err = fmt.Sscanf(file.Path, "datadog/2/AGENT_CONFIG/gen-%d/config.json", &pathGen)
				if !assert.NoError(t, err, "unparseable file path: %s", file.Path) {
					continue
				}

				expectedContent := fmt.Sprintf("content-for-generation-%d", pathGen)
				assert.Equal(t, expectedContent, string(file.Raw),
					"file path (generation %d) and content disagree -- response mixed two generations", pathGen)
				assert.Equal(t, metaGen, pathGen,
					"Targets metadata (generation %d) and returned file (generation %d) disagree -- response mixed two generations", metaGen, pathGen)
			}
		}()
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()

	require.Zero(t, mismatches.Load(), "observed non-monotonic snapshot generations")
	t.Logf("observed %d non-empty, internally-consistent responses across %d readers", responses.Load(), numReaders)
}
