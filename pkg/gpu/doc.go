// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

/*
Package gpu contains code for monitoring of GPUs from system probe. The data flow is as follows:

 1. The main entry point is the Probe type in probe.go, which sets up the eBPF uprobes for library functions.
 2. Library uprobes send events each time those functions are called through an event ringbuffer.
 3. The consumer.go file contains the CUDA event consumer, which reads events from the ringbuffer and sends them to the appropriate stream handler.
 4. The stream.go file contains the StreamHandler type, which processes events and generates data for a specific stream of GPU commands.
 5. When the probe receives a data request via the GetAndFlush method, it calls onto the statsGenerator (stags.go) to generate the GPU stats that will be sent.
 6. The statsGenerator takes the data from all active stream handlers, and distributes them to the appropriate aggregators.
 7. The aggregators (aggregator.go) process data from multiple streams from a single process, and generate process-level stats.
*/
package gpu
