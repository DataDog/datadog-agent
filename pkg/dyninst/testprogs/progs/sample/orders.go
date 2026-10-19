// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"fmt"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

type Request struct {
	ID       int
	Customer string
	Total    float64
}

type Server struct {
	name string
}

const testOrdersSpanID = uint64(0xfedcba9876543210)

//go:noinline
func (s *Server) handleOrder(ctx context.Context, req *Request) error {
	if req.Total < 0 {
		return fmt.Errorf("server %s: order %d for %s has negative total %v", s.name, req.ID, req.Customer, req.Total)
	}
	fmt.Println("handled order", req.ID, req.Customer, req.Total)
	return ctx.Err()
}

func executeServerFuncs(ctx context.Context) {
	span, ctx := tracer.StartSpanFromContext(ctx, "sample.orders", tracer.WithSpanID(testOrdersSpanID))
	defer span.Finish()

	s := &Server{name: "orders"}
	s.handleOrder(ctx, &Request{ID: 1, Customer: "Alice", Total: 42.50})
}
