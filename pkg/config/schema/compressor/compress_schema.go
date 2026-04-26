// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// We use the same version of ZSTD than the Agent.
package main

import (
	"log"
	"os"

	"github.com/DataDog/zstd"
)

func main() {
	args := os.Args[1:]
	if len(args)%2 != 0 {
		log.Fatal("usage: compress_schema <input> <output> [<input> <output> ...]")
	}

	for i := 0; i < len(args); i += 2 {
		input, output := args[i], args[i+1]

		data, err := os.ReadFile(input)
		if err != nil {
			log.Fatal(err)
		}
		compressed, err := zstd.Compress(nil, data)
		if err != nil {
			log.Fatal(err)
		}

		err = os.WriteFile(output, compressed, 0600)
		if err != nil {
			log.Fatal(err)
		}
	}
}
