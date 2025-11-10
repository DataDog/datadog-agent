// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package decode is responsible for decoding raw event bytes from dynamic
// instrumentation's ebpf ringbuffer into JSON to be uploaded to the datadog
// logs backend.
//
// JSON is written directly to the decoder's output stream for efficiency.
// An example of the JSON output is:
//
//	{
//	    "service": "sample",
//	    "ddsource": "dd_debugger",
//	    "logger": {
//	        "name": "",
//	        "method": "",
//	        "version": 1,
//	    },
//	    "debugger": {
//	        "snapshot": {
//	            "id": "6550c768-2210-47bd-9597-5ad10c96986b",
//	            "timestamp": 1750978383964,
//	            "language": "go",
//	            "stack": [
//	                {
//	                    "function": "main.testSingleInt",
//	                    "fileName": "/home/vagrant/datadog-agent/pkg/dyninst/testprogs/progs/sample/basics.go",
//	                    "lineNumber": 23
//	                },
//	                {
//	                    "function": "main.executeBasicFuncs",
//	                    "fileName": "/home/vagrant/datadog-agent/pkg/dyninst/testprogs/progs/sample/basics.go",
//	                    "lineNumber": 92
//	                },
//	                {
//	                    "function": "main.main",
//	                    "fileName": "/home/vagrant/datadog-agent/pkg/dyninst/testprogs/progs/sample/main.go",
//	                    "lineNumber": 24
//	                },
//	                {
//	                    "function": "runtime.main",
//	                    "fileName": "/usr/local/go/src/runtime/proc.go",
//	                    "lineNumber": 283
//	                },
//	                {
//	                    "function": "runtime.goexit",
//	                    "fileName": "/usr/local/go/src/runtime/asm_arm64.s",
//	                    "lineNumber": 1223
//	                }
//	            ],
//	            "probe": {
//	                "id": "testSingleInt",
//	                "location": {
//	                    "method": "main.testSingleInt"
//	                }
//	            },
//	            "captures": {
//	                "entry": {
//	                    "arguments": {
//	                        "x": {
//	                            "type": "int",
//	                            "value": -1512
//	                        }
//	                    }
//	                }
//	            }
//	        }
//	    }
//	}
package decode
