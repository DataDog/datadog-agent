// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startstop

import "fmt"

type component struct {
	ch   chan string
	name string
}

func newComponent(ch chan string, name string) *component {
	return &component{ch, name}
}

func newComponents(num int) (ch chan string, components []*component) {
	ch = make(chan string, 20) // large enough to cache all events in a test
	for i := 0; i < num; i++ {
		name := fmt.Sprintf("c%d", i)
		components = append(components, newComponent(ch, name))
	}
	return
}

func (c *component) Start() {
	c.ch <- "start " + c.name
}

func (c *component) Stop() {
	c.ch <- "stop " + c.name
}
