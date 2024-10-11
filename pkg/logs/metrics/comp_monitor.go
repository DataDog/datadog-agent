// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

type Size interface {
	Size() int64
}

var TlmIngressBytes = telemetry.NewCounter("logs_component", "ingress_bytes", []string{"name", "instance"}, "")
var TlmEgressBytes = telemetry.NewCounter("logs_component", "egress_bytes", []string{"name", "instance"}, "")

type CompMonitor[T Size] struct {
	buffer   chan T
	input    chan T
	name     string
	instance string
	// ingressBytes int64
	// egressBytes  int64
}

// func (c *CompMonitor[T]) addIngressBytes(n int64) {
// 	atomic.AddInt64(&c.ingressBytes, n)
// }

// func (c *CompMonitor[T]) addEgressBytes(n int64) {
// 	atomic.AddInt64(&c.egressBytes, n)
// }

// func (c *CompMonitor[T]) GetIngressBytes() int64 {
// 	return atomic.LoadInt64(&c.ingressBytes)
// }

// func (c *CompMonitor[T]) GetEgressBytes() int64 {
// 	return atomic.LoadInt64(&c.egressBytes)
// }

func NewCompMonitor[T Size](bufferSize int, name, instance string) *CompMonitor[T] {
	c := &CompMonitor[T]{
		buffer:   make(chan T, bufferSize),
		input:    make(chan T),
		name:     name,
		instance: instance,
	}
	go c.run()
	return c

}

func (c *CompMonitor[T]) run() {
	go func() {
		for m := range c.input {
			c.buffer <- m
			TlmIngressBytes.Add(float64(m.Size()), c.name, c.instance)
		}
		close(c.buffer)
	}()
}

func (c *CompMonitor[T]) GetInputChan() chan T {
	return c.input
}

func (c *CompMonitor[T]) RecvChan() chan T {
	return c.buffer
}

func (c *CompMonitor[T]) ReportEgress(size Size) {
	TlmEgressBytes.Add(float64(size.Size()), c.name, c.instance)
}

func (c *CompMonitor[T]) Close() {
	close(c.input)
}
