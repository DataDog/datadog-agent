// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// e2ectl-worker is the pulumi-executor: the piece of code where we accept to
// import Pulumi run functions, so the rest of the CLI never pays Pulumi's
// price. It executes the scenarios registered in scenarios.go: provision
// infrastructure and optional fakeintake, write the snapshot, and destroy
// the stack. Agent installation and iteration happen outside Pulumi in the
// core. Local environments remain independent of this executor, including
// their local Docker fakeintake.
//
// The engine is generic and forever-static (executor.go): parse the job,
// look the scenario up by base, strict-decode the params, provision or
// destroy. New environments add a scenario file and one registration line.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--protocol-version" {
		fmt.Println(workerclient.ProtocolVersion)
		return
	}
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: e2ectl-worker <job.json>")
		os.Exit(2)
	}
	var j workerclient.Job
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fatal("reading job: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		fatal("parsing job: %v", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		fatal("expected a single executor job")
	}
	if err := runJob(j); err != nil {
		fatal("%v", err)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "e2ectl-worker: "+format+"\n", args...)
	os.Exit(1)
}
