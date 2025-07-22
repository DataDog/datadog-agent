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
		expected      funcName
	}{
		{"simple func", "github.com/cockroachdb/cockroach/pkg/kv.func1",
			funcName{
				Package: "github.com/cockroachdb/cockroach/pkg/kv",
				Type:    "",
				Name:    "func1",
			},
		},
		{"method", "github.com/cockroachdb/cockroach/pkg/kv/kvserver.raftSchedulerShard.worker",
			funcName{
				Package: "github.com/cockroachdb/cockroach/pkg/kv/kvserver",
				Type:    "raftSchedulerShard",
				Name:    "worker",
			},
		},
		{"method with ptr receiver", "github.com/cockroachdb/cockroach/pkg/kv/kvserver.(*raftSchedulerShard).worker",
			funcName{
				Package: "github.com/cockroachdb/cockroach/pkg/kv/kvserver",
				Type:    "raftSchedulerShard",
				Name:    "worker",
			},
		},
		{"generic function", "os.init.OnceValue[go.shape.interface { Error() string }].func3",
			funcName{
				GenericFunction: true,
			},
		},
		{"anonymous function inside function", "github.com/getsentry/sentry-go.NewClient.func1",
			funcName{
				Package: "github.com/getsentry/sentry-go",
				Type:    "",
				Name:    "NewClient.func1",
			},
		},
		{"anonymous function in ptr method", "github.com/cockroachdb/cockroach/pkg/sql/physicalplan/replicaoracle.preferFollowerOracle.(*ChoosePreferredReplica).func1",
			// This function we would like to parse, but currently we confuse it with
			// an anonymous function called by an inlined function, which we don't
			// support parsing.
			funcName{},
			// Ideally, we would parse it as:
			//funcName{
			//	Package: "github.com/cockroachdb/cockroach/pkg/sql/physicalplan/replicaoracle",
			//	Type:    "preferFollowerOracle",
			//	Name:    "ChoosePreferredReplica.func1",
			//},
		},
		{"anonymous function in val method", "github.com/cockroachdb/cockroach/pkg/sql/physicalplan/replicaoracle.preferFollowerOracle.ChoosePreferredReplica.func1",
			// This function we would like to parse, but currently we confuse it with
			// an anonymous function called by an inlined function, which we don't
			// support parsing.
			funcName{},
			// Ideally, we would parse it as:
			//funcName{
			//	Package: "github.com/cockroachdb/cockroach/pkg/sql/physicalplan/replicaoracle",
			//	Type:    "preferFollowerOracle",
			//	Name:    "ChoosePreferredReplica.func1",
			//},
		},
		{"deeply nested function", "github.com/cockroachdb/cockroach/pkg/server.(*apiV2Server).execSQL.func8.1.3.2",
			// This function we would like to parse, but currently we confuse it with
			// an anonymous function called by an inlined function, which we don't
			// support parsing.
			funcName{},
			// Ideally, we would parse it as:
			//funcName{
			//	Package: "github.com/cockroachdb/cockroach/pkg/server",
			//	Type:    "apiV2Server",
			//	Name:    "execSQL.func8.1.3.2",
			//},
		},
		{"deeply nested deferwrap", "github.com/foo/logical.(*logicalReplicationWriterProcessor).flushBuffer.Group.GoCtx.func7.1.deferwrap1",
			funcName{
				Package: "github.com/foo/logical",
				Type:    "logicalReplicationWriterProcessor",
				Name:    "flushBuffer.Group.GoCtx.func7.1.deferwrap1",
			},
		},
		{"funky function called by inlined function", "runtime.gcMarkDone.forEachP.func5",
			// We don't support parsing such functions.
			funcName{},
		},
		{"funky function called by inlined function", "time.map.init.0",
			// We don't support parsing such functions.
			funcName{},
		},
		{"funky function called by inlined function inside inlined function", "runtime.chansend.send.goready.func2",
			// We don't support parsing such functions.
			funcName{},
		},
		{"funky method called by inlined function", "github.com/cockroachdb/cockroach/pkg/server.(*topLevelServer).startPersistingHLCUpperBound.func1.(*Node).SetHLCUpperBound.1",
			// We don't support parsing such functions.
			funcName{},
		},
		{"funky method called by inlined function 2", "github.com/cockroachdb/cockroach/pkg/crosscluster/producer.(*spanConfigEventStream).startStreamProcessor.(*spanConfigEventStream).startStreamProcessor.func1.func6",
			// We don't support parsing such functions.
			funcName{},
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
			if !tt.expected.Empty() {
				tt.expected.QualifiedName = tt.qualifiedName
			}
			require.Equal(t, tt.expected, result)
		})
	}
}
