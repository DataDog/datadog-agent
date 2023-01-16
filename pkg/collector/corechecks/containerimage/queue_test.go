// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package containerimage

import (
	"strconv"
	"sync"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/contimage"
	"github.com/stretchr/testify/assert"
)

func newMockFlush() (callback func([]*model.ContainerImage), getAccumulator func() [][]*model.ContainerImage) {
	accumulator := [][]*model.ContainerImage{}
	var mutex sync.RWMutex

	callback = func(images []*model.ContainerImage) {
		mutex.Lock()
		defer mutex.Unlock()
		accumulator = append(accumulator, images)
	}

	getAccumulator = func() [][]*model.ContainerImage {
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
		queue <- &model.ContainerImage{
			Id: strconv.Itoa(i),
		}
	}

	assert.Equal(
		t,
		accumulator(),
		[][]*model.ContainerImage{
			{{Id: "0"}, {Id: "1"}, {Id: "2"}},
			{{Id: "3"}, {Id: "4"}, {Id: "5"}},
			{{Id: "6"}, {Id: "7"}, {Id: "8"}},
		},
	)

	time.Sleep(100 * time.Millisecond)

	assert.Equal(
		t,
		accumulator(),
		[][]*model.ContainerImage{
			{{Id: "0"}, {Id: "1"}, {Id: "2"}},
			{{Id: "3"}, {Id: "4"}, {Id: "5"}},
			{{Id: "6"}, {Id: "7"}, {Id: "8"}},
			{{Id: "9"}, {Id: "10"}},
		},
	)

	close(queue)
}
