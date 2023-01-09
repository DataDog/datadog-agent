// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sbom

import (
	"strconv"
	"sync"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/sbom"
	"github.com/stretchr/testify/assert"
)

func newMockFlush() (callback func([]*model.SBOMEntity), getAccumulator func() [][]*model.SBOMEntity) {
	accumulator := [][]*model.SBOMEntity{}
	var mutex sync.RWMutex

	callback = func(sbom []*model.SBOMEntity) {
		mutex.Lock()
		defer mutex.Unlock()
		accumulator = append(accumulator, sbom)
	}

	getAccumulator = func() [][]*model.SBOMEntity {
		mutex.RLock()
		defer mutex.RUnlock()
		return accumulator
	}

	return
}

func TestQueue(t *testing.T) {
	callback, accumulator := newMockFlush()
	queue := newQueue(3, 50*time.Millisecond, callback)

	for i := 0; i <= 10; i++ {
		queue <- &model.SBOMEntity{
			Id: strconv.Itoa(i),
		}
	}

	assert.Equal(
		t,
		accumulator(),
		[][]*model.SBOMEntity{
			{{Id: "0"}, {Id: "1"}, {Id: "2"}},
			{{Id: "3"}, {Id: "4"}, {Id: "5"}},
			{{Id: "6"}, {Id: "7"}, {Id: "8"}},
		},
	)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(
		t,
		accumulator(),
		[][]*model.SBOMEntity{
			{{Id: "0"}, {Id: "1"}, {Id: "2"}},
			{{Id: "3"}, {Id: "4"}, {Id: "5"}},
			{{Id: "6"}, {Id: "7"}, {Id: "8"}},
			{{Id: "9"}, {Id: "10"}},
		},
	)

	close(queue)
}
