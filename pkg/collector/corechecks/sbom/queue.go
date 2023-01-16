// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sbom

import (
	"time"

	model "github.com/DataDog/agent-payload/v5/sbom"
)

type queue struct {
	maxNbItem        int
	maxRetentionTime time.Duration
	flushCB          func([]*model.SBOMEntity)
	enqueueCh        chan *model.SBOMEntity
	data             []*model.SBOMEntity
	timer            *time.Timer
}

// newQueue returns a chan to enqueue newly discovered container images
func newQueue(maxNbItem int, maxRetentionTime time.Duration, flushCB func([]*model.SBOMEntity)) chan *model.SBOMEntity {
	q := queue{
		maxNbItem:        maxNbItem,
		maxRetentionTime: maxRetentionTime,
		flushCB:          flushCB,
		enqueueCh:        make(chan *model.SBOMEntity),
		data:             make([]*model.SBOMEntity, 0, maxNbItem),
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
			case sbom, more := <-q.enqueueCh:
				if !more {
					return
				}
				q.enqueue(sbom)
			}
		}
	}()

	return q.enqueueCh
}

func (q *queue) enqueue(elem *model.SBOMEntity) {
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
	q.data = make([]*model.SBOMEntity, 0, q.maxNbItem)
}
