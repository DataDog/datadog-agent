/* SPDX-License-Identifier: BSD-2-Clause */

package main

import (
	"encoding/json"
	"fmt"
	"go/build"
	"io"
	"log"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/dublintraceroute/results"
)

func init() {
	// Ensure that CGO is disabled
	var ctx build.Context
	if ctx.CgoEnabled {
		fmt.Println("Disabling CGo")
		ctx.CgoEnabled = false
	}
}

var (
	flagOutputFile = flag.StringP("output", "o", "-", "Output file. Use \"-\" to print to standard output")
)

func main() {

	flag.Parse()

	var (
		buf []byte
		err error
	)
	if len(flag.Args()) == 0 || flag.Arg(0) == "-" {
		fmt.Fprintf(os.Stderr, "Reading from stdin...\n")
		buf, err = io.ReadAll(os.Stdin)
	} else {
		buf, err = os.ReadFile(flag.Arg(0))
	}
	if err != nil {
		log.Fatalf("Failed to read file '%s': %v", flag.Arg(0), err)
	}

	var result results.Results
	if err := json.Unmarshal(buf, &result); err != nil {
		log.Fatalf("Failed to unmarshal JSON into Results: %v", err)
	}
	output, err := result.ToDOT()
	if err != nil {
		log.Fatalf("Failed to convert to DOT: %v", err)
	}
	if *flagOutputFile == "-" {
		fmt.Println(output)
	} else {
		err := os.WriteFile(*flagOutputFile, []byte(output), 0644)
		if err != nil {
			log.Fatalf("Failed to write DOT file: %v", err)
		}
		log.Printf("Saved DOT file to %s", *flagOutputFile)
		log.Printf("Run `dot -Tpng \"%s\" -o \"%s.png\"` to convert to PNG", *flagOutputFile, *flagOutputFile)
	}
}
