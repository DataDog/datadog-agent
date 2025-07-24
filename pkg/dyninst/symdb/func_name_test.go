// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package symdb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFuncName(t *testing.T) {
	tests := []struct {
		testName      string
		qualifiedName string
		expected      parseFuncNameResult
	}{
		{"simple func", "github.com/cockroachdb/cockroach/pkg/kv.func1",
			parseFuncNameResult{funcName: funcName{
				Package: "github.com/cockroachdb/cockroach/pkg/kv",
				Type:    "",
				Name:    "func1",
			}},
		},
		{"method", "github.com/cockroachdb/cockroach/pkg/kv/kvserver.raftSchedulerShard.worker",
			parseFuncNameResult{funcName: funcName{
				Package: "github.com/cockroachdb/cockroach/pkg/kv/kvserver",
				Type:    "raftSchedulerShard",
				Name:    "worker",
			}},
		},
		{"method with ptr receiver", "github.com/cockroachdb/cockroach/pkg/kv/kvserver.(*raftSchedulerShard).worker",
			parseFuncNameResult{funcName: funcName{
				Package: "github.com/cockroachdb/cockroach/pkg/kv/kvserver",
				Type:    "raftSchedulerShard",
				Name:    "worker",
			}},
		},
		{"generic function", "os.init.OnceValue[go.shape.interface { Error() string }].func3",
			parseFuncNameResult{
				failureReason: parseFuncNameFailureReasonGenericFunction,
			},
		},
		{"anonymous function defined inside free-standing function", "github.com/getsentry/sentry-go.NewClient.func1",
			parseFuncNameResult{funcName: funcName{
				Package: "github.com/getsentry/sentry-go",
				Type:    "",
				Name:    "NewClient.func1",
			}},
		},
		{"anonymous function defined inside free-standing function 2", "github.com/klauspost/compress/flate.init.0",
			parseFuncNameResult{funcName: funcName{
				Package: "github.com/klauspost/compress/flate",
				Type:    "",
				Name:    "init.0",
			}},
		},
		{"anonymous function defined inside method with pointer receiver", "github.com/cockroachdb/pebble/wal.(*FailoverOptions).EnsureDefaults.func1",
			parseFuncNameResult{funcName: funcName{
				Package: "github.com/cockroachdb/pebble/wal",
				Name:    "FailoverOptions.EnsureDefaults.func1",
			}},
		},
		{"anonymous function defined inside method with value receiver", "github.com/cockroachdb/pebble/wal.FailoverOptions.EnsureDefaults.func1",
			parseFuncNameResult{funcName: funcName{
				Package: "github.com/cockroachdb/pebble/wal",
				Type:    "",
				Name:    "FailoverOptions.EnsureDefaults.func1",
			}},
		},
		{"deeply nested function", "github.com/cockroachdb/cockroach/pkg/server.(*apiV2Server).execSQL.func8.1.3.2",
			parseFuncNameResult{funcName: funcName{
				Package: "github.com/cockroachdb/cockroach/pkg/server",
				Type:    "",
				Name:    "apiV2Server.execSQL.func8.1.3.2",
			}},
		},
		{"deeply nested deferwrap", "github.com/foo/logical.(*logicalReplicationWriterProcessor).flushBuffer.Group.GoCtx.func7.1.deferwrap1",
			parseFuncNameResult{funcName: funcName{
				Package: "github.com/foo/logical",
				Type:    "",
				Name:    "logicalReplicationWriterProcessor.flushBuffer.Group.GoCtx.func7.1.deferwrap1",
			}},
		},
		{"funky function called by inlined function", "runtime.gcMarkDone.forEachP.func5",
			parseFuncNameResult{funcName: funcName{
				Package: "runtime",
				Type:    "",
				Name:    "gcMarkDone.forEachP.func5",
			}},
		},
		{"funky function called by inlined function inside inlined function", "runtime.chansend.send.goready.func2",
			parseFuncNameResult{funcName: funcName{
				Package: "runtime",
				Type:    "",
				Name:    "chansend.send.goready.func2",
			}},
		},
		{"funky method called by inlined function", "github.com/cockroachdb/cockroach/pkg/server.(*topLevelServer).startPersistingHLCUpperBound.func1.(*Node).SetHLCUpperBound.1",
			parseFuncNameResult{funcName: funcName{
				Package: "github.com/cockroachdb/cockroach/pkg/server",
				Type:    "",
				Name:    "topLevelServer.startPersistingHLCUpperBound.func1.(*Node).SetHLCUpperBound.1",
			}},
		},
		{"funky method called by inlined function 2", "github.com/cockroachdb/cockroach/pkg/crosscluster/producer.(*spanConfigEventStream).startStreamProcessor.(*spanConfigEventStream).startStreamProcessor.func1.func6",
			parseFuncNameResult{funcName: funcName{
				Package: "github.com/cockroachdb/cockroach/pkg/crosscluster/producer",
				Type:    "",
				Name:    "spanConfigEventStream.startStreamProcessor.(*spanConfigEventStream).startStreamProcessor.func1.func6",
			}},
		},
		{"static map initializer", "time.map.init.0",
			// We don't support parsing such functions.
			parseFuncNameResult{
				failureReason: parseFuncNameFailureReasonMapInit,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			result, err := parseFuncName(tt.qualifiedName)
			if err != nil {
				t.Fatal(err)
			}
			// The test cases don't fill in the expected QualifiedName field;
			// fix it up.
			if tt.expected.failureReason == parseFuncNameFailureReasonUndefined {
				tt.expected.funcName.QualifiedName = tt.qualifiedName
			}
			require.Equal(t, tt.expected, result)
		})
	}
}
