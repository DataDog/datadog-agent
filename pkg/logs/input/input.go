// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package input

import (
	"sync"
)

// Input represents a log collector
type Input interface {
	Start()
	Stop()
}

// Inputs holds a collection of inputs
type Inputs struct {
	inputs []Input
}

// NewInputs returns a new Inputs
func NewInputs(inputs []Input) *Inputs {
	return &Inputs{inputs}
}

// Start starts all single input in sequence
func (i *Inputs) Start() {
	for _, input := range i.inputs {
		input.Start()
	}
}

// Stop stops all inputs in parallel
func (i *Inputs) Stop() {
	wg := &sync.WaitGroup{}
	for _, input := range i.inputs {
		wg.Add(1)
		go func(i Input) {
			i.Stop()
			wg.Done()
		}(input)
	}
	wg.Wait()
}
