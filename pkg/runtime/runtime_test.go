// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
