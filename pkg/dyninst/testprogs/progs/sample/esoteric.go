// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type something interface {
	Do()
}

type somethingImpl struct {
}

func (s *somethingImpl) Do() {}

type esotericStack struct {
	_      int32
	_valid int32
	c      chan struct{}
	val    any
	_      uint64
	work   something
	foo    func()
}

type esotericHeap struct {
	esotericStack
	mu   sync.Mutex
	once sync.Once
	wg   sync.WaitGroup
	a    atomic.Int32
}

//go:noinline
func testEsotericStack(e esotericStack) {
	fmt.Printf("%#+v\n", e)
}

//go:noinline
func testEsotericHeap(e *esotericHeap) {
	fmt.Printf("%#+v\n", e)
}

func executeEsoteric() {
	capture := 0
	esotericStack := esotericStack{
		_valid: 77,
		c:      make(chan struct{}),
		val:    42,
		work:   &somethingImpl{},
		foo:    func() { capture += 1 },
	}
	testEsotericStack(esotericStack)
	esotericHeap := &esotericHeap{
		esotericStack: esotericStack,
		mu:            sync.Mutex{},
		once:          sync.Once{},
		wg:            sync.WaitGroup{},
		a:             atomic.Int32{},
	}
	testEsotericHeap(esotericHeap)
}
