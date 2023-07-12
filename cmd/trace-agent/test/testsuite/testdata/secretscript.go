// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

// This script is a dummy emulating the behavior of a secret command used in the Datadog Agent configuration
// as the value of the environment variable "DD_SECRET_BACKEND_COMMAND" which mirrors the YAML config setting
// "secret_backend_command".
//
// It takes whatever secret keys it is sent and resolves them to the same string as the key, prefixed with
// "decrypted_". For example, requesting the secret "secret1" will resolve to "decrypted_secret1".
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
)

type secretsPayload struct {
	Secrets []string `json:secrets`
	Version string   `json:version`
}

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not read from stdin: %s", err)
		os.Exit(1)
	}

	secrets := secretsPayload{}
	if err := json.Unmarshal(data, &secrets); err != nil {
		log.Fatal(err)
	}

	res := map[string]map[string]string{}
	for _, handle := range secrets.Secrets {
		res[handle] = map[string]string{
			"value": "decrypted_" + handle,
		}
	}

	output, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not serialize res: %s", err)
		os.Exit(1)
	}
	fmt.Printf(string(output))

	f, err := os.OpenFile("/tmp/secrets.out", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(output, '\n')); err != nil {
		log.Fatal(err)
	}
}
