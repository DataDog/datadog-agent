// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"time"
	"unsafe"
)

var buffer [(1 << 30) + 1024]byte

type Param struct {
	idx    uint32
	random uint32
}

//go:noinline
func ping(param *Param) {
	fmt.Printf("ping %#v %#v\n", binary.LittleEndian.AppendUint32(nil, param.idx), binary.LittleEndian.AppendUint32(nil, param.random))
}

func pinger(i int, rate_hz float64) {
	r := rand.New(rand.NewSource(int64(i)))
	target := time.Now()
	iter := uint32(0)
	for {
		iter += 1
		delta := r.ExpFloat64() / rate_hz
		random := r.Uint32()
		target = target.Add(time.Duration(delta * float64(time.Second)))
		time.Sleep(time.Until(target))
		ping(&Param{idx: iter, random: random})
	}
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: sample [concurrency] [rate_hz]")
		return
	}

	for i, max := 0, len(buffer)/1024; i < max; i++ {
		buffer[i*1024] = 0
	}
	fmt.Printf("buffer location %d\n", uintptr(unsafe.Pointer(&buffer[0])))

	concurrency, err := strconv.Atoi(os.Args[1])
	if err != nil {
		panic((err))
	}
	rate_hz, err := strconv.ParseFloat(os.Args[2], 64)
	if err != nil {
		panic((err))
	}

	for i := 0; i < concurrency; i++ {
		go pinger(i, rate_hz)
	}
	fmt.Println("[READY]")

	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)
	<-ctx.Done()
}
