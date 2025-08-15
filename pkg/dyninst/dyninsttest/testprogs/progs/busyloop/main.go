// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Busyloop is a sample go program that can be used for benchmarking.
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"
)

//go:noinline
func noop() {}

func busyloop(concurrency int, duration time.Duration) float64 {
	ready := make(chan struct{})
	start := make(chan struct{})
	stop := make(chan struct{})
	counts := make(chan int)
	for i := 0; i < concurrency; i++ {
		go func() {
			iters := 0
			// Synchronize all goroutines after threads have been spawned.
			ready <- struct{}{}
			<-start
		loop:
			for {
				select {
				case <-stop:
					break loop
				default:
					// Busy loop.
					noop()
					iters++
				}
			}
			counts <- iters
		}()
	}
	for i := 0; i < concurrency; i++ {
		<-ready
	}
	close(start)
	time.Sleep(duration)
	close(stop)
	total := 0
	for i := 0; i < concurrency; i++ {
		total += <-counts
	}
	return 1e6 * float64(concurrency) / float64(total)
}

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: ./busyloop [round_cnt] [round_sec] [concurrency]")
		return
	}
	// Wait for input signalling that loop should be started.
	round_cnt, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	round_sec, err := strconv.Atoi(os.Args[2])
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	concurrency, err := strconv.Atoi(os.Args[3])
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	_, err = fmt.Scanln()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	results := make([]float64, round_cnt)
	avg := 0.0
	for i := 0; i < round_cnt; i++ {
		results[i] = busyloop(concurrency, time.Duration(round_sec)*time.Second)
		fmt.Printf("%.6fus\n", results[i])
		avg += results[i]
	}
	avg /= float64(len(results))
	stddev := 0.0
	for i := 0; i < round_cnt; i++ {
		stddev += (results[i] - avg) * (results[i] - avg)
	}
	stddev /= float64(len(results))
	stddev = math.Sqrt(stddev)
	fmt.Printf("avg=%.6fus stddev=%.6fus\n", avg, stddev)
}
