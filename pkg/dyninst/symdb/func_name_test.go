package symdb_test

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestParseFuncName(t *testing.T) {
	tests := []struct {
		testName      string
		qualifiedName string
		expected      symdb.FuncName
	}{
		{"simple func", "github.com/cockroachdb/cockroach/pkg/kv.func1",
			symdb.FuncName{
				Package: "github.com/cockroachdb/cockroach/pkg/kv",
				Type:    "",
				Name:    "func1",
			},
		},
		{"method", "github.com/cockroachdb/cockroach/pkg/kv/kvserver.raftSchedulerShard.worker",
			symdb.FuncName{
				Package: "github.com/cockroachdb/cockroach/pkg/kv/kvserver",
				Type:    "raftSchedulerShard",
				Name:    "worker",
			},
		},
		{"method with ptr receiver", "github.com/cockroachdb/cockroach/pkg/kv/kvserver.(*raftSchedulerShard).worker",
			symdb.FuncName{
				Package: "github.com/cockroachdb/cockroach/pkg/kv/kvserver",
				Type:    "raftSchedulerShard",
				Name:    "worker",
			},
		},
		{"generic function", "os.init.OnceValue[go.shape.interface { Error() string }].func3",
			symdb.FuncName{
				GenericFunction: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			result, err := symdb.ParseFuncName(tt.qualifiedName)
			if err != nil {
				t.Fatal(err)
			}
			tt.expected.QualifiedName = tt.qualifiedName
			require.Equal(t, tt.expected, result)
		})
	}
}
