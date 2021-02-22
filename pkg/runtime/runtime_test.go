package runtime

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAutoMaxProcs(t *testing.T) {

	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(0))

	// let's change at runtime to 2 threads
	runtime.GOMAXPROCS(2)
	assert.Equal(t, 2, runtime.GOMAXPROCS(0))

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
