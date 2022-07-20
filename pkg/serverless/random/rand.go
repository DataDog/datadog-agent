// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package random

import (
	cryptorand "crypto/rand"
	"math"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"


// Random holds a thread-safe source of random numbers.
var Random *rand.Rand

func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\serverless\random\rand.go 22`)
	var seed int64
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\serverless\random\rand.go 23`)
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(math.MaxInt64))
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\serverless\random\rand.go 24`)
	if err == nil {
		seed = n.Int64()
	} else {
		log.Warnf("cannot generate random seed: %v; using current time", err)
		seed = time.Now().UnixNano()
	}
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\serverless\random\rand.go 30`)
	Random = rand.New(&safeSource{
		source: rand.NewSource(seed),
	})
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\serverless\random\rand.go 33`)
}

// safeSource holds a thread-safe implementation of rand.Source64.
type safeSource struct {
	source rand.Source
	sync.Mutex
}

func (rs *safeSource) Int63() int64 {
	rs.Lock()
	n := rs.source.Int63()
	rs.Unlock()

	return n
}

func (rs *safeSource) Uint64() uint64 { return uint64(rs.Int63()) }

func (rs *safeSource) Seed(seed int64) {
	rs.Lock()
	rs.source.Seed(seed)
	rs.Unlock()
}