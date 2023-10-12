// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package hash holds hash related files
package hash

import (
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
)

func generateFileData(size int) []byte {
	var out []byte
	for i := 0; i < size; i++ {
		out = append(out, byte('a'))
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
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA1, model.SHA256, model.MD5},
				HashResolverMaxHashRate:    1,
				HashResolverMaxHashBurst:   1,
				HashResolverMaxFileSize:    1 << 20,
			},
			args: args{
				event: &model.Event{
					BaseEvent: model.BaseEvent{
						FieldHandlers: &model.DefaultFieldHandlers{},
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
				"sha1:3495ff69d34671d1e15b33a63c1379fdedd3a32a",
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
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA1, model.SHA256, model.MD5},
				HashResolverMaxHashRate:    1,
				HashResolverMaxHashBurst:   1,
				HashResolverMaxFileSize:    1 << 20,
			},
			args: args{
				event: &model.Event{
					BaseEvent: model.BaseEvent{
						FieldHandlers: &model.DefaultFieldHandlers{},
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
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA1, model.SHA256, model.MD5},
				HashResolverMaxHashRate:    1,
				HashResolverMaxHashBurst:   1,
				HashResolverMaxFileSize:    1 << 10,
			},
			args: args{
				event: &model.Event{
					BaseEvent: model.BaseEvent{
						FieldHandlers: &model.DefaultFieldHandlers{},
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
				"sha1:8eca554631df9ead14510e1a70ae48c70f9b9384",
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
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA1, model.SHA256, model.MD5},
				HashResolverMaxHashRate:    1,
				HashResolverMaxHashBurst:   1,
				HashResolverMaxFileSize:    1 << 10,
			},
			args: args{
				event: &model.Event{
					BaseEvent: model.BaseEvent{
						FieldHandlers: &model.DefaultFieldHandlers{},
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
				HashResolverHashAlgorithms: []model.HashAlgorithm{model.SHA1, model.SHA256, model.MD5},
				HashResolverMaxHashRate:    0,
				HashResolverMaxHashBurst:   0,
				HashResolverMaxFileSize:    1 << 10,
			},
			args: args{
				event: &model.Event{
					BaseEvent: model.BaseEvent{
						FieldHandlers: &model.DefaultFieldHandlers{},
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
			got := resolver.ComputeHashesFromEvent(tt.args.event, tt.args.file)
			if !reflect.DeepEqual(strings.Join(got, "-"), strings.Join(tt.want, "-")) {
				t.Errorf("ComputeHashes() = %v, want %v", got, tt.want)
			}
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
				HashResolverMaxHashBurst:   math.MaxInt,
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
				HashResolverMaxHashBurst:   math.MaxInt,
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
				HashResolverMaxHashBurst:   math.MaxInt,
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

	for _, bb := range benchmarks {
		for _, fc := range bb.fileSizes {
			b.Run(bb.name+"/"+fc.name, func(caseB *testing.B) {
				caseB.Helper()

				// reset file
				f, err := os.Create("/tmp/hash_bench")
				if err != nil {
					caseB.Errorf("couldn't create benchmark file: %v", err)
					return
				}
				if _, err = f.Write(generateFileData(fc.fileSize)); err != nil {
					caseB.Errorf("couldn't write file content: %v", err)
					return
				}
				_ = f.Close()

				resolver, err := NewResolver(bb.config, client, nil)
				if err != nil {
					b.Fatalf("couldn't instantiate a new hash resolver: %v", err)
				}

				caseB.ResetTimer()

				for i := 0; i < caseB.N; i++ {
					got := resolver.ComputeHashesFromEvent(&model.Event{
						BaseEvent: model.BaseEvent{
							FieldHandlers: &model.DefaultFieldHandlers{},
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
						PathnameStr:           "/tmp/hash_bench",
						IsPathnameStrResolved: true,
					})
					if len(got) == 0 {
						caseB.Errorf("hash computation failed (due to rate limiting ?): got %v", got)
					}
				}
			})
		}
	}

	// delete test file
	if err := os.Remove("/tmp/hash_bench"); err != nil {
		b.Errorf("couldn't delete benchmark file: %v", err)
	}
}
