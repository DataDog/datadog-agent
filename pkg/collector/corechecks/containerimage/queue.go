// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package containerimage

import (
	"time"

	model "github.com/DataDog/agent-payload/v5/contimage"
)

type queue struct {
	maxNbItem        int
	maxRetentionTime time.Duration
	flushCB          func([]*model.ContainerImage)
	enqueueCh        chan *model.ContainerImage
	data             []*model.ContainerImage
	timer            *time.Timer
}

// newQueue returns a chan to enqueue newly discovered container images
func newQueue(maxNbItem int, maxRetentionTime time.Duration, flushCB func([]*model.ContainerImage)) chan *model.ContainerImage {
	q := queue{
		maxNbItem:        maxNbItem,
		maxRetentionTime: maxRetentionTime,
		flushCB:          flushCB,
		enqueueCh:        make(chan *model.ContainerImage),
		data:             make([]*model.ContainerImage, 0, maxNbItem),
		timer:            time.NewTimer(maxRetentionTime),
	}

	if !q.timer.Stop() {
		<-q.timer.C
	}

	go func() {
		for {
			select {
			case <-q.timer.C:
				q.flush()
			case img, more := <-q.enqueueCh:
				if !more {
					return
				}
				q.enqueue(img)
			}
		}
	}()

	return q.enqueueCh
}

func (q *queue) enqueue(elem *model.ContainerImage) {
	if len(q.data) == 0 {
		q.timer.Reset(q.maxRetentionTime)
	}

	q.data = append(q.data, elem)

	if len(q.data) == q.maxNbItem {
		q.flush()
	}
}

func (q *queue) flush() {
	q.timer.Stop()
	q.flushCB(q.data)
	q.data = make([]*model.ContainerImage, 0, q.maxNbItem)
}
