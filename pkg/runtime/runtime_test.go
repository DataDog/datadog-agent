package runtime

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/require"
)

func TestAutoMaxProcs(t *testing.T) {

	assert.Equal(t, runtime.NumCPU(), runtime.GOMAXPROCS(0))

	os.Setenv("GOMAXPROCS", "1000m")
	// set new limit
	SetMaxProcs()
	assert.Equal(t, 1, runtime.GOMAXPROCS(0))

	os.Setenv("GOMAXPROCS", "1500m")
	// set new limit
	SetMaxProcs()
	assert.Equal(t, 1, runtime.GOMAXPROCS(0))

	os.Setenv("GOMAXPROCS", "2000m")
	// set new limit
	SetMaxProcs()
	assert.Equal(t, 2, runtime.GOMAXPROCS(0))
}
