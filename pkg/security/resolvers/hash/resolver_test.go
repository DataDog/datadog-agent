// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package hash holds hash related files
package hash

import (
	"bytes"
	"math"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/glaslos/ssdeep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fips"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
)

// TestNewResolver_FIPSExcludesSHA1 asserts that the hash resolver silently drops sha1 from its
// configured algorithms when the agent is built for FIPS, since sha1 is not a FIPS-approved algorithm.
func TestNewResolver_FIPSExcludesSHA1(t *testing.T) {
	client := statsdclient.NewStatsdClient()
	cfg := &config.RuntimeSecurityConfig{
		HashResolverEnabled:        true,
		HashResolverEventTypes:     []model.EventType{model.ExecEventType},
		HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA1, model.SHA256},
		HashResolverMaxHashRate:    1,
		HashResolverMaxFileSize:    1 << 20,
	}

	resolver, err := NewResolver(cfg, client, nil)
	require.NoError(t, err)

	if fips.BuiltForFIPS() {
		assert.NotContains(t, resolver.opts.HashAlgorithms, model.SHA1, "sha1 should be excluded when built for FIPS")
	} else {
		assert.Contains(t, resolver.opts.HashAlgorithms, model.SHA1)
	}
}

func generateFileData(size int) []byte {
	var out []byte
	for i := 0; i < size; i++ {
		out = append(out, byte('a'))
	}
	return out
}

func generateFileDataWithPattern(size int, pattern byte) []byte {
	var out []byte
	for i := 0; i < size; i++ {
		out = append(out, byte(pattern+byte(i%256)))
	}
	return out
}

func TestResolver_ComputeHashes(t *testing.T) {
	type args struct {
		event    *model.Event
		file     *model.FileEvent
		fileSize int
	}
	client := statsdclient.NewStatsdClient()
	pid := uint32(os.Getpid())
	tests := []struct {
		name          string
		config        *config.RuntimeSecurityConfig
		args          args
		want          []string
		wantHashState model.HashState
	}{
		{
			name: "event_type/ok",
			config: &config.RuntimeSecurityConfig{
				HashResolverEnabled:        true,
				HashResolverEventTypes:     []model.EventType{model.ExecEventType},
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA256, model.MD5},
				HashResolverMaxHashRate:    1,
				HashResolverMaxFileSize:    1 << 20,
			},
			args: args{
				event: &model.Event{
					BaseEvent: model.BaseEvent{
						FieldHandlers: &model.FakeFieldHandlers{},
						Type:          uint32(model.ExecEventType),
						ProcessContext: &model.ProcessContext{
							Process: model.Process{
								PIDContext: model.PIDContext{
									Pid: pid,
								},
							},
						},
					},
				},
				file: &model.FileEvent{
					PathnameStr:           "/tmp/hash_test",
					IsPathnameStrResolved: true,
				},
				fileSize: 10,
			},
			want: []string{
				"sha256:bf2cb58a68f684d95a3b78ef8f661c9a4e5b09e82cc8f9cc88cce90528caeb27",
				"md5:e09c80c42fda55f9d992e59ca6b3307d",
			},
			wantHashState: model.Done,
		},
		{
			name: "event_type/event_type_not_hashed",
			config: &config.RuntimeSecurityConfig{
				HashResolverEnabled:        true,
				HashResolverEventTypes:     []model.EventType{model.FileOpenEventType},
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA256, model.MD5},
				HashResolverMaxHashRate:    1,
				HashResolverMaxFileSize:    1 << 20,
			},
			args: args{
				event: &model.Event{
					BaseEvent: model.BaseEvent{
						FieldHandlers: &model.FakeFieldHandlers{},
						Type:          uint32(model.ExecEventType),
						ProcessContext: &model.ProcessContext{
							Process: model.Process{
								PIDContext: model.PIDContext{
									Pid: pid,
								},
							},
						},
					},
				},
				file: &model.FileEvent{
					PathnameStr:           "/tmp/hash_test",
					IsPathnameStrResolved: true,
				},
				fileSize: 0,
			},
			want:          []string{},
			wantHashState: model.EventTypeNotConfigured,
		},
		{
			name: "max_file_size/ok",
			config: &config.RuntimeSecurityConfig{
				HashResolverEnabled:        true,
				HashResolverEventTypes:     []model.EventType{model.ExecEventType},
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA256, model.MD5},
				HashResolverMaxHashRate:    1,
				HashResolverMaxFileSize:    1 << 10,
			},
			args: args{
				event: &model.Event{
					BaseEvent: model.BaseEvent{
						FieldHandlers: &model.FakeFieldHandlers{},
						Type:          uint32(model.ExecEventType),
						ProcessContext: &model.ProcessContext{
							Process: model.Process{
								PIDContext: model.PIDContext{
									Pid: pid,
								},
							},
						},
					},
				},
				file: &model.FileEvent{
					PathnameStr:           "/tmp/hash_test",
					IsPathnameStrResolved: true,
				},
				fileSize: 1 << 10,
			},
			want: []string{
				"sha256:2edc986847e209b4016e141a6dc8716d3207350f416969382d431539bf292e4a",
				"md5:c9a34cfc85d982698c6ac89f76071abd",
			},
			wantHashState: model.Done,
		},
		{
			name: "max_file_size/file_too_big",
			config: &config.RuntimeSecurityConfig{
				HashResolverEnabled:        true,
				HashResolverEventTypes:     []model.EventType{model.ExecEventType},
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA256, model.MD5},
				HashResolverMaxHashRate:    1,
				HashResolverMaxFileSize:    1 << 10,
			},
			args: args{
				event: &model.Event{
					BaseEvent: model.BaseEvent{
						FieldHandlers: &model.FakeFieldHandlers{},
						Type:          uint32(model.ExecEventType),
						ProcessContext: &model.ProcessContext{
							Process: model.Process{
								PIDContext: model.PIDContext{
									Pid: pid,
								},
							},
						},
					},
				},
				file: &model.FileEvent{
					PathnameStr:           "/tmp/hash_test",
					IsPathnameStrResolved: true,
				},
				fileSize: 1<<10 + 1,
			},
			want:          []string{},
			wantHashState: model.FileTooBig,
		},
		{
			name: "rate_limit",
			config: &config.RuntimeSecurityConfig{
				HashResolverEnabled:        true,
				HashResolverEventTypes:     []model.EventType{model.ExecEventType},
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA256, model.MD5},
				HashResolverMaxHashRate:    0,
				HashResolverMaxFileSize:    1 << 10,
			},
			args: args{
				event: &model.Event{
					BaseEvent: model.BaseEvent{
						FieldHandlers: &model.FakeFieldHandlers{},
						Type:          uint32(model.ExecEventType),
						ProcessContext: &model.ProcessContext{
							Process: model.Process{
								PIDContext: model.PIDContext{
									Pid: pid,
								},
							},
						},
					},
				},
				file: &model.FileEvent{
					PathnameStr:           "/tmp/hash_test",
					IsPathnameStrResolved: true,
				},
				fileSize: 1,
			},
			want:          []string{},
			wantHashState: model.HashWasRateLimited,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.Create(tt.args.file.PathnameStr)
			if err != nil {
				t.Errorf("couldn't create test file: %v", err)
				return
			}
			if _, err = f.Write(generateFileData(tt.args.fileSize)); err != nil {
				t.Errorf("couldn't write file content: %v", err)
				return
			}
			_ = f.Close()

			resolver, err := NewResolver(tt.config, client, nil)
			if err != nil {
				t.Fatalf("couldn't instantiate a new hash resolver: %v", err)
			}
			got := resolver.ComputeHashesFromEvent(tt.args.event, tt.args.file, 0)
			assert.ElementsMatch(t, tt.want, got, "invalid output hashes")
			assert.Equal(t, tt.wantHashState, tt.args.file.HashState, "invalid output hash state")
		})
	}

	// delete test file
	if err := os.Remove("/tmp/hash_test"); err != nil {
		t.Errorf("couldn't delete test file: %v", err)
	}
}

// ---------------------------
// | Output of the benchmark |
// ---------------------------
//
// BenchmarkHashFunctions/sha1/1Kb-16         	   71650	     14919 ns/op	   43294 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha1/10Kb-16        	   54848	     21157 ns/op	   43298 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha1/100Kb-16       	   10000	    117873 ns/op	   43295 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha1/500Kb-16       	    2355	    494134 ns/op	   43295 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha1/1Mb-16         	    1177	    990903 ns/op	   43292 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha1/5Mb-16         	    1238	    967194 ns/op	   43295 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha1/10Mb-16        	     100	  10457671 ns/op	   43278 B/op	      24 allocs/op (current default configuration of the agent)
// BenchmarkHashFunctions/sha1/20Mb-16        	      57	  21141343 ns/op	   43296 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha1/50Mb-16        	      19	  52905197 ns/op	   43376 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha1/100Mb-16       	      10	 106099397 ns/op	   43485 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha1/500Mb-16       	       2	 531379180 ns/op	   44404 B/op	      26 allocs/op
// BenchmarkHashFunctions/sha256/1Kb-16       	   68380	     14981 ns/op	   43391 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha256/10Kb-16      	   35421	     33088 ns/op	   43394 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha256/100Kb-16     	    5566	    230064 ns/op	   43391 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha256/500Kb-16     	     969	   1081308 ns/op	   43395 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha256/1Mb-16       	     554	   2141840 ns/op	   43395 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha256/5Mb-16       	     556	   2194208 ns/op	   43391 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha256/10Mb-16      	      54	  22621965 ns/op	   43394 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha256/20Mb-16      	      25	  45000456 ns/op	   43443 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha256/50Mb-16      	       9	 116726777 ns/op	   43607 B/op	      24 allocs/op
// BenchmarkHashFunctions/sha256/100Mb-16     	       5	 222499288 ns/op	   43811 B/op	      25 allocs/op
// BenchmarkHashFunctions/sha256/500Mb-16     	       1	1157351629 ns/op	   45648 B/op	      29 allocs/op
// BenchmarkHashFunctions/md5/1Kb-16          	   74629	     13649 ns/op	   43236 B/op	      24 allocs/op
// BenchmarkHashFunctions/md5/10Kb-16         	   51951	     23567 ns/op	   43240 B/op	      24 allocs/op
// BenchmarkHashFunctions/md5/100Kb-16        	    9411	    159144 ns/op	   43236 B/op	      24 allocs/op
// BenchmarkHashFunctions/md5/500Kb-16        	    2030	    556789 ns/op	   43240 B/op	      24 allocs/op
// BenchmarkHashFunctions/md5/1Mb-16          	    1051	   1133978 ns/op	   43240 B/op	      24 allocs/op
// BenchmarkHashFunctions/md5/5Mb-16          	     962	   1125095 ns/op	   43239 B/op	      24 allocs/op
// BenchmarkHashFunctions/md5/10Mb-16         	     100	  11712801 ns/op	   43220 B/op	      24 allocs/op
// BenchmarkHashFunctions/md5/20Mb-16         	      44	  23195419 ns/op	   43249 B/op	      24 allocs/op
// BenchmarkHashFunctions/md5/50Mb-16         	      18	  61665791 ns/op	   43324 B/op	      24 allocs/op
// BenchmarkHashFunctions/md5/100Mb-16        	       9	 119887060 ns/op	   43452 B/op	      24 allocs/op
// BenchmarkHashFunctions/md5/500Mb-16        	       2	 620456840 ns/op	   44348 B/op	      26 allocs/op

func TestSSDeepCaching(t *testing.T) {
	client := statsdclient.NewStatsdClient()
	pid := uint32(os.Getpid())

	// Create a test file
	tmpFile, err := os.CreateTemp("", "ssdeep_cache_test")
	if err != nil {
		t.Fatalf("couldn't create test file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	size := 8192

	// Write some data (SSDEEP requires at least 4KB typically)
	testData := generateFileDataWithPattern(size, 'a')
	if _, err := tmpFile.Write(testData); err != nil {
		t.Fatalf("couldn't write file content: %v", err)
	}
	tmpFile.Close()

	// Create resolver with MD5 and SSDEEP
	config := &config.RuntimeSecurityConfig{
		HashResolverEnabled:        true,
		HashResolverEventTypes:     []model.EventType{model.ExecEventType},
		HashResolverHashAlgorithms: []model.HashAlgorithm{model.MD5, model.SSDEEP},
		HashResolverMaxHashRate:    math.MaxInt,
		HashResolverMaxFileSize:    math.MaxInt64,
		HashResolverCacheSize:      100,
	}

	resolver, err := NewResolver(config, client, nil)
	if err != nil {
		t.Fatalf("couldn't instantiate a new hash resolver: %v", err)
	}

	// First computation - should compute both MD5 and SSDEEP
	event := &model.Event{
		BaseEvent: model.BaseEvent{
			FieldHandlers: &model.FakeFieldHandlers{},
			Type:          uint32(model.ExecEventType),
			ProcessContext: &model.ProcessContext{
				Process: model.Process{
					PIDContext: model.PIDContext{
						Pid: pid,
					},
				},
			},
		},
	}

	file := &model.FileEvent{
		PathnameStr:           tmpFile.Name(),
		IsPathnameStrResolved: true,
	}

	hashes1 := resolver.ComputeHashesFromEvent(event, file, 0)
	assert.Equal(t, model.Done, file.HashState, "first computation should succeed")
	assert.Equal(t, 2, len(hashes1), "should have 2 hashes")

	// Extract MD5 and SSDEEP from first computation
	var md5Hash1, ssdeepHash1 string
	for _, hash := range hashes1 {
		if strings.HasPrefix(hash, "md5:") {
			md5Hash1 = hash
		} else if strings.HasPrefix(hash, "ssdeep:") {
			ssdeepHash1 = hash
		}
	}
	assert.NotEmpty(t, md5Hash1, "should have MD5 hash")
	assert.NotEmpty(t, ssdeepHash1, "should have SSDEEP hash")

	// Verify SSDEEP is cached with MD5 as key
	if resolver.ssdeepCache != nil {
		ssdeepKey := SSDeepCacheKey{cheapHash: md5Hash1, size: int64(size)}
		cached, ok := resolver.ssdeepCache.Get(ssdeepKey)
		assert.True(t, ok, "SSDEEP should be cached with MD5 as key")
		assert.Equal(t, ssdeepHash1, cached.ssdeepHash, "cached SSDEEP should match")
	}

	// Second computation on same file - SSDEEP should be retrieved from cache
	file2 := &model.FileEvent{
		PathnameStr:           tmpFile.Name(),
		IsPathnameStrResolved: true,
	}

	hashes2 := resolver.ComputeHashesFromEvent(event, file2, 0)
	assert.Equal(t, 2, len(hashes2), "should have 2 hashes")

	var md5Hash2, ssdeepHash2 string
	for _, hash := range hashes2 {
		if strings.HasPrefix(hash, "md5:") {
			md5Hash2 = hash
		} else if strings.HasPrefix(hash, "ssdeep:") {
			ssdeepHash2 = hash
		}
	}

	assert.Equal(t, md5Hash1, md5Hash2, "MD5 should be the same")
	assert.Equal(t, ssdeepHash1, ssdeepHash2, "SSDEEP should be the same (from cache)")

	// Modify the file with a significantly different pattern
	f, err := os.OpenFile(tmpFile.Name(), os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("couldn't open file for modification: %v", err)
	}

	size = 16384

	newData := generateFileDataWithPattern(size, 'z')
	if _, err := f.Write(newData); err != nil {
		t.Fatalf("couldn't write new content: %v", err)
	}
	f.Close()

	// Third computation - file changed, both hashes should be recomputed
	file3 := &model.FileEvent{
		PathnameStr:           tmpFile.Name(),
		IsPathnameStrResolved: true,
	}

	hashes3 := resolver.ComputeHashesFromEvent(event, file3, 0)
	assert.Equal(t, 2, len(hashes3), "should have 2 hashes")

	var md5Hash3, ssdeepHash3 string
	for _, hash := range hashes3 {
		if strings.HasPrefix(hash, "md5:") {
			md5Hash3 = hash
		} else if strings.HasPrefix(hash, "ssdeep:") {
			ssdeepHash3 = hash
		}
	}

	assert.NotEqual(t, md5Hash1, md5Hash3, "MD5 should be different after file change")
	assert.NotEqual(t, ssdeepHash1, ssdeepHash3, "SSDEEP should be different after file change")

	// Verify new SSDEEP is cached with new MD5
	if resolver.ssdeepCache != nil {
		ssdeepKey := SSDeepCacheKey{cheapHash: md5Hash3, size: int64(size)}
		cached, ok := resolver.ssdeepCache.Get(ssdeepKey)
		assert.True(t, ok, "new SSDEEP should be cached with new MD5 as key")
		assert.Equal(t, ssdeepHash3, cached.ssdeepHash, "cached SSDEEP should match new hash")
	}
}

func TestCrossCgroupCacheHit(t *testing.T) {
	client := statsdclient.NewStatsdClient()
	pid := uint32(os.Getpid())

	tmpFile, err := os.CreateTemp("", "cross_cgroup_cache_test")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(generateFileDataWithPattern(8192, 'b'))
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	cfg := &config.RuntimeSecurityConfig{
		HashResolverEnabled:        true,
		HashResolverEventTypes:     []model.EventType{model.ExecEventType},
		HashResolverHashAlgorithms: []model.HashAlgorithm{model.MD5, model.SSDEEP},
		HashResolverMaxHashRate:    1,
		HashResolverMaxFileSize:    math.MaxInt64,
		HashResolverCacheSize:      100,
	}

	resolver, err := NewResolver(cfg, client, nil)
	require.NoError(t, err)

	newEvent := func(cgroupID string) *model.Event {
		return &model.Event{
			BaseEvent: model.BaseEvent{
				FieldHandlers: &model.FakeFieldHandlers{},
				Type:          uint32(model.ExecEventType),
				ProcessContext: &model.ProcessContext{
					Process: model.Process{
						PIDContext: model.PIDContext{Pid: pid},
						CGroup: model.CGroupContext{
							CGroupID: containerutils.CGroupID(cgroupID),
						},
					},
				},
			},
		}
	}

	file1 := &model.FileEvent{
		FileFields:            model.FileFields{PathKey: model.PathKey{Inode: 4242, PathID: 1}},
		PathnameStr:           tmpFile.Name(),
		IsPathnameStrResolved: true,
	}
	hashes1 := resolver.ComputeHashesFromEvent(newEvent("cgroup-1"), file1, 0)
	require.Equal(t, model.Done, file1.HashState)
	require.Len(t, hashes1, 2)

	file2 := &model.FileEvent{
		FileFields:            model.FileFields{PathKey: model.PathKey{Inode: 4242, PathID: 2}},
		PathnameStr:           tmpFile.Name(),
		IsPathnameStrResolved: true,
	}
	hashes2 := resolver.ComputeHashesFromEvent(newEvent("cgroup-2"), file2, 0)
	require.Equal(t, model.Done, file2.HashState)
	assert.Equal(t, hashes1, hashes2)
}

func TestHashRateLimiterRejectsWhenExceeded(t *testing.T) {
	client := statsdclient.NewStatsdClient()
	pid := uint32(os.Getpid())

	tmpDir := t.TempDir()
	f1Path := tmpDir + "/file1"
	f2Path := tmpDir + "/file2"
	require.NoError(t, os.WriteFile(f1Path, []byte("file1"), 0o600))
	require.NoError(t, os.WriteFile(f2Path, []byte("file2"), 0o600))

	cfg := &config.RuntimeSecurityConfig{
		HashResolverEnabled:        true,
		HashResolverEventTypes:     []model.EventType{model.ExecEventType},
		HashResolverHashAlgorithms: []model.HashAlgorithm{model.MD5},
		HashResolverMaxHashRate:    1,
		HashResolverMaxFileSize:    1 << 10,
		HashResolverCacheSize:      100,
	}

	resolver, err := NewResolver(cfg, client, nil)
	require.NoError(t, err)

	ev := &model.Event{
		BaseEvent: model.BaseEvent{
			FieldHandlers: &model.FakeFieldHandlers{},
			Type:          uint32(model.ExecEventType),
			ProcessContext: &model.ProcessContext{
				Process: model.Process{
					PIDContext: model.PIDContext{Pid: pid},
				},
			},
		},
	}

	file1 := &model.FileEvent{PathnameStr: f1Path, IsPathnameStrResolved: true}
	hashes1 := resolver.ComputeHashesFromEvent(ev, file1, 0)
	require.Equal(t, model.Done, file1.HashState)
	require.Len(t, hashes1, 1)

	file2 := &model.FileEvent{PathnameStr: f2Path, IsPathnameStrResolved: true}
	hashes2 := resolver.ComputeHashesFromEvent(ev, file2, 0)
	require.Equal(t, model.HashWasRateLimited, file2.HashState)
	require.Empty(t, hashes2)
}

func TestSSDeepSkipsSmallFile(t *testing.T) {
	client := statsdclient.NewStatsdClient()
	pid := uint32(os.Getpid())

	tmpFile, err := os.CreateTemp("", "ssdeep_small_file_test")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(generateFileDataWithPattern(minSSDeepFileSize, 'c'))
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	cfg := &config.RuntimeSecurityConfig{
		HashResolverEnabled:        true,
		HashResolverEventTypes:     []model.EventType{model.ExecEventType},
		HashResolverHashAlgorithms: []model.HashAlgorithm{model.MD5, model.SSDEEP},
		HashResolverMaxHashRate:    math.MaxInt,
		HashResolverMaxFileSize:    math.MaxInt64,
		HashResolverCacheSize:      100,
	}

	resolver, err := NewResolver(cfg, client, nil)
	require.NoError(t, err)

	event := &model.Event{
		BaseEvent: model.BaseEvent{
			FieldHandlers: &model.FakeFieldHandlers{},
			Type:          uint32(model.ExecEventType),
			ProcessContext: &model.ProcessContext{
				Process: model.Process{
					PIDContext: model.PIDContext{Pid: pid},
				},
			},
		},
	}

	file := &model.FileEvent{
		PathnameStr:           tmpFile.Name(),
		IsPathnameStrResolved: true,
	}
	hashes := resolver.ComputeHashesFromEvent(event, file, 0)
	require.Equal(t, model.Done, file.HashState)
	require.Len(t, hashes, 1)
	assert.True(t, strings.HasPrefix(hashes[0], "md5:"))
}

func TestFastSSDeepHasherMatchesUpstream(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	cases := []struct {
		name string
		data []byte
	}{
		{
			name: "patterned_8kb",
			data: generateFileDataWithPattern(8192, 'a'),
		},
		{
			name: "patterned_64kb",
			data: generateFileDataWithPattern(65536, 'z'),
		},
		{
			name: "random_10kb",
			data: func() []byte {
				b := make([]byte, 10240)
				rng.Read(b)
				return b
			}(),
		},
		{
			name: "random_256kb_with_runs",
			data: func() []byte {
				b := make([]byte, 256*1024)
				rng.Read(b)
				for i := 1000; i < len(b)-100; i += 4096 {
					for j := 0; j < 32; j++ {
						b[i+j] = 'X'
					}
				}
				return b
			}(),
		},
		{
			name: "low_entropy_zeroes_8kb",
			data: bytes.Repeat([]byte{0}, 8192),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := ssdeep.New()
			_, err := upstream.Write(tc.data)
			require.NoError(t, err)
			want := upstream.Sum(nil)

			fast := newFastSSDeepHasher()
			for off := 0; off < len(tc.data); off += 1337 {
				end := min(off+1337, len(tc.data))
				_, err := fast.Write(tc.data[off:end])
				require.NoError(t, err)
			}
			got := fast.Sum(nil)
			assert.Equal(t, string(want), string(got))
		})
	}
}

func TestSSDeepCrossInodeAndNegativeCache(t *testing.T) {
	client := statsdclient.NewStatsdClient()
	pid := uint32(os.Getpid())

	tmpFile1, err := os.CreateTemp("", "ssdeep_cross_inode_1")
	require.NoError(t, err)
	defer os.Remove(tmpFile1.Name())

	tmpFile2, err := os.CreateTemp("", "ssdeep_cross_inode_2")
	require.NoError(t, err)
	defer os.Remove(tmpFile2.Name())

	payload := generateFileDataWithPattern(8192, 'k')
	_, err = tmpFile1.Write(payload)
	require.NoError(t, err)
	require.NoError(t, tmpFile1.Close())

	_, err = tmpFile2.Write(payload)
	require.NoError(t, err)
	require.NoError(t, tmpFile2.Close())

	cfg := &config.RuntimeSecurityConfig{
		HashResolverEnabled:        true,
		HashResolverEventTypes:     []model.EventType{model.ExecEventType},
		HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA256, model.SSDEEP},
		HashResolverMaxHashRate:    math.MaxInt,
		HashResolverMaxFileSize:    math.MaxInt64,
		HashResolverCacheSize:      100,
	}

	resolver, err := NewResolver(cfg, client, nil)
	require.NoError(t, err)

	event := &model.Event{
		BaseEvent: model.BaseEvent{
			FieldHandlers: &model.FakeFieldHandlers{},
			Type:          uint32(model.ExecEventType),
			ProcessContext: &model.ProcessContext{
				Process: model.Process{
					PIDContext: model.PIDContext{Pid: pid},
				},
			},
		},
	}

	file1 := &model.FileEvent{
		PathnameStr:           tmpFile1.Name(),
		IsPathnameStrResolved: true,
	}
	file1.Inode = 1001
	file2 := &model.FileEvent{
		PathnameStr:           tmpFile2.Name(),
		IsPathnameStrResolved: true,
	}
	file2.Inode = 2002

	hashes1 := resolver.ComputeHashesFromEvent(event, file1, 0)
	hashes2 := resolver.ComputeHashesFromEvent(event, file2, 0)
	assert.Equal(t, hashes1, hashes2)
	assert.Equal(t, 1, resolver.ssdeepCache.Len())
}

func BenchmarkHashFunctions(b *testing.B) {
	client := statsdclient.NewStatsdClient()
	pid := uint32(os.Getpid())
	type fileCase struct {
		name     string
		fileSize int
	}
	benchmarks := []struct {
		name      string
		config    *config.RuntimeSecurityConfig
		fileSizes []fileCase
	}{
		{
			name: "sha1",
			config: &config.RuntimeSecurityConfig{
				HashResolverEnabled:        true,
				HashResolverEventTypes:     []model.EventType{model.ExecEventType},
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA1},
				HashResolverMaxHashRate:    math.MaxInt,
				HashResolverMaxFileSize:    math.MaxInt64,
			},
			fileSizes: []fileCase{
				{
					name:     "1Kb",
					fileSize: 1 << 10,
				},
				{
					name:     "10Kb",
					fileSize: (1 << 10) * 10,
				},
				{
					name:     "100Kb",
					fileSize: (1 << 10) * 100,
				},
				{
					name:     "500Kb",
					fileSize: (1 << 10) * 500,
				},
				{
					name:     "1Mb",
					fileSize: 1 << 20,
				},
				{
					name:     "5Mb",
					fileSize: 1 << 20,
				},
				{
					name:     "10Mb",
					fileSize: (1 << 20) * 10,
				},
				{
					name:     "20Mb",
					fileSize: (1 << 20) * 20,
				},
				{
					name:     "50Mb",
					fileSize: (1 << 20) * 50,
				},
				{
					name:     "100Mb",
					fileSize: (1 << 20) * 100,
				},
				{
					name:     "500Mb",
					fileSize: (1 << 20) * 500,
				},
			},
		},
		{
			name: "sha256",
			config: &config.RuntimeSecurityConfig{
				HashResolverEnabled:        true,
				HashResolverEventTypes:     []model.EventType{model.ExecEventType},
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA256},
				HashResolverMaxHashRate:    math.MaxInt,
				HashResolverMaxFileSize:    math.MaxInt64,
			},
			fileSizes: []fileCase{
				{
					name:     "1Kb",
					fileSize: 1 << 10,
				},
				{
					name:     "10Kb",
					fileSize: (1 << 10) * 10,
				},
				{
					name:     "100Kb",
					fileSize: (1 << 10) * 100,
				},
				{
					name:     "500Kb",
					fileSize: (1 << 10) * 500,
				},
				{
					name:     "1Mb",
					fileSize: 1 << 20,
				},
				{
					name:     "5Mb",
					fileSize: 1 << 20,
				},
				{
					name:     "10Mb",
					fileSize: (1 << 20) * 10,
				},
				{
					name:     "20Mb",
					fileSize: (1 << 20) * 20,
				},
				{
					name:     "50Mb",
					fileSize: (1 << 20) * 50,
				},
				{
					name:     "100Mb",
					fileSize: (1 << 20) * 100,
				},
				{
					name:     "500Mb",
					fileSize: (1 << 20) * 500,
				},
			},
		},
		{
			name: "md5",
			config: &config.RuntimeSecurityConfig{
				HashResolverEnabled:        true,
				HashResolverEventTypes:     []model.EventType{model.ExecEventType},
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.MD5},
				HashResolverMaxHashRate:    math.MaxInt,
				HashResolverMaxFileSize:    math.MaxInt64,
			},
			fileSizes: []fileCase{
				{
					name:     "1Kb",
					fileSize: 1 << 10,
				},
				{
					name:     "10Kb",
					fileSize: (1 << 10) * 10,
				},
				{
					name:     "100Kb",
					fileSize: (1 << 10) * 100,
				},
				{
					name:     "500Kb",
					fileSize: (1 << 10) * 500,
				},
				{
					name:     "1Mb",
					fileSize: 1 << 20,
				},
				{
					name:     "5Mb",
					fileSize: 1 << 20,
				},
				{
					name:     "10Mb",
					fileSize: (1 << 20) * 10,
				},
				{
					name:     "20Mb",
					fileSize: (1 << 20) * 20,
				},
				{
					name:     "50Mb",
					fileSize: (1 << 20) * 50,
				},
				{
					name:     "100Mb",
					fileSize: (1 << 20) * 100,
				},
				{
					name:     "500Mb",
					fileSize: (1 << 20) * 500,
				},
			},
		},
	}

	tmpDir := b.TempDir()

	for _, bb := range benchmarks {
		for _, fc := range bb.fileSizes {
			b.Run(bb.name+"/"+fc.name, func(caseB *testing.B) {
				caseB.Helper()

				// reset file
				f, err := os.CreateTemp(tmpDir, "hash_bench")
				if err != nil {
					caseB.Errorf("couldn't create benchmark file: %v", err)
					return
				}
				if _, err := f.Write(generateFileData(fc.fileSize)); err != nil {
					caseB.Errorf("couldn't write file content: %v", err)
					return
				}
				if err := f.Close(); err != nil {
					caseB.Errorf("couldn't close benchmark file: %v", err)
					return
				}

				resolver, err := NewResolver(bb.config, client, nil)
				if err != nil {
					b.Fatalf("couldn't instantiate a new hash resolver: %v", err)
				}

				caseB.ResetTimer()

				for i := 0; i < caseB.N; i++ {
					got := resolver.ComputeHashesFromEvent(&model.Event{
						BaseEvent: model.BaseEvent{
							FieldHandlers: &model.FakeFieldHandlers{},
							Type:          uint32(model.ExecEventType),
							ProcessContext: &model.ProcessContext{
								Process: model.Process{
									PIDContext: model.PIDContext{
										Pid: pid,
									},
								},
							},
						},
					}, &model.FileEvent{
						PathnameStr:           f.Name(),
						IsPathnameStrResolved: true,
					}, 0)
					if len(got) == 0 {
						caseB.Errorf("hash computation failed (due to rate limiting ?): got %v", got)
					}
				}
			})
		}
	}
}

func TestComputeHashesFromEventAsyncAndFlush(t *testing.T) {
	client := statsdclient.NewStatsdClient()
	pid := uint32(os.Getpid())

	tmpFile, err := os.CreateTemp("", "async_hash_test")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(generateFileDataWithPattern(8192, 'a'))
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	cfg := &config.RuntimeSecurityConfig{
		HashResolverEnabled:        true,
		HashResolverEventTypes:     []model.EventType{model.ExecEventType},
		HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA256, model.MD5, model.SSDEEP},
		HashResolverMaxHashRate:    math.MaxInt,
		HashResolverMaxFileSize:    math.MaxInt64,
		HashResolverCacheSize:      100,
		HashResolverReplace: map[string]string{
			"/replaced/path": "sha256:replaceddigest",
		},
	}

	resolver, err := NewResolver(cfg, client, nil)
	require.NoError(t, err)

	newEvent := func(evtType model.EventType) *model.Event {
		return &model.Event{
			BaseEvent: model.BaseEvent{
				FieldHandlers: &model.FakeFieldHandlers{},
				Type:          uint32(evtType),
				ProcessContext: &model.ProcessContext{
					Process: model.Process{
						PIDContext: model.PIDContext{Pid: pid},
					},
				},
			},
		}
	}

	// Fast path 1: unconfigured event type resolves synchronously (async == false)
	var fastState model.HashState
	var fastAsync bool
	unconfiguredFile := &model.FileEvent{PathnameStr: tmpFile.Name(), IsPathnameStrResolved: true}
	resolver.ComputeHashesFromEventAsync(newEvent(model.FileOpenEventType), unconfiguredFile, 0, func(state model.HashState, _ []string, async bool) {
		fastState = state
		fastAsync = async
	})
	assert.False(t, fastAsync)
	assert.Equal(t, model.EventTypeNotConfigured, fastState)

	// Fast path 2: replace map resolves synchronously (async == false)
	var replaceHashes []string
	replaceFile := &model.FileEvent{PathnameStr: "/replaced/path", IsPathnameStrResolved: true}
	resolver.ComputeHashesFromEventAsync(newEvent(model.ExecEventType), replaceFile, 0, func(state model.HashState, hashes []string, async bool) {
		fastState = state
		replaceHashes = hashes
		fastAsync = async
	})
	assert.False(t, fastAsync)
	assert.Equal(t, model.Done, fastState)
	assert.Equal(t, []string{"sha256:replaceddigest"}, replaceHashes)

	// Async path: real file is hashed on the background worker and synchronized by Flush()
	var (
		mu          sync.Mutex
		asyncState  model.HashState
		asyncHashes []string
		wasAsync    bool
	)
	asyncFile := &model.FileEvent{PathnameStr: tmpFile.Name(), IsPathnameStrResolved: true}
	resolver.ComputeHashesFromEventAsync(newEvent(model.ExecEventType), asyncFile, 0, func(state model.HashState, hashes []string, async bool) {
		mu.Lock()
		defer mu.Unlock()
		asyncState = state
		asyncHashes = hashes
		wasAsync = async
	})

	resolver.Flush()

	mu.Lock()
	gotState := asyncState
	gotHashes := asyncHashes
	gotWasAsync := wasAsync
	mu.Unlock()

	assert.True(t, gotWasAsync)
	assert.Equal(t, model.Done, gotState)
	assert.Len(t, gotHashes, 3)

	// Verify synchronous ComputeHashesFromEvent produces the exact same hashes from cache
	syncFile := &model.FileEvent{PathnameStr: tmpFile.Name(), IsPathnameStrResolved: true}
	syncHashes := resolver.ComputeHashesFromEvent(newEvent(model.ExecEventType), syncFile, 0)
	assert.Equal(t, gotHashes, syncHashes)

	// Fast path 3: cached file with matching Inode and MTime resolves synchronously (async == false)
	_, _, mtime, fkey, err := getFileInfo(tmpFile.Name())
	require.NoError(t, err)
	cachedAsyncFile := &model.FileEvent{
		PathnameStr:           tmpFile.Name(),
		IsPathnameStrResolved: true,
	}
	cachedAsyncFile.Inode = fkey.inode
	cachedAsyncFile.MTime = mtime
	var cachedHashes []string
	resolver.ComputeHashesFromEventAsync(newEvent(model.ExecEventType), cachedAsyncFile, 0, func(state model.HashState, hashes []string, async bool) {
		fastState = state
		cachedHashes = hashes
		fastAsync = async
	})
	assert.False(t, fastAsync)
	assert.Equal(t, model.Done, fastState)
	assert.Equal(t, model.Done, cachedAsyncFile.HashState)
	assert.Equal(t, gotHashes, cachedHashes)
	assert.Equal(t, gotHashes, cachedAsyncFile.Hashes)
}

func TestConcurrentAsyncAndSyncComputeHashes(t *testing.T) {
	client := statsdclient.NewStatsdClient()
	pid := uint32(os.Getpid())

	tmpFile, err := os.CreateTemp("", "concurrent_async_hash_test")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(generateFileDataWithPattern(8192, 'c'))
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	cfg := &config.RuntimeSecurityConfig{
		HashResolverEnabled:        true,
		HashResolverEventTypes:     []model.EventType{model.ExecEventType},
		HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA256, model.MD5, model.SSDEEP},
		HashResolverMaxHashRate:    math.MaxInt,
		HashResolverMaxFileSize:    math.MaxInt64,
		HashResolverCacheSize:      100,
	}

	resolver, err := NewResolver(cfg, client, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				ev := &model.Event{
					BaseEvent: model.BaseEvent{
						FieldHandlers: &model.FakeFieldHandlers{},
						Type:          uint32(model.ExecEventType),
						ProcessContext: &model.ProcessContext{
							Process: model.Process{
								PIDContext: model.PIDContext{Pid: pid},
							},
						},
					},
				}
				fe := &model.FileEvent{PathnameStr: tmpFile.Name(), IsPathnameStrResolved: true}
				resolver.ComputeHashesFromEventAsync(ev, fe, 0, func(state model.HashState, hashes []string, _ bool) {
					assert.Equal(t, model.Done, state)
					assert.Len(t, hashes, 3)
				})
			}
			resolver.Flush()
		}()
	}
	wg.Wait()
}
