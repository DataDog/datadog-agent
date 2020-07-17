package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Input struct {
	Version string   `json:"version"`
	Secrets []string `json:"secrets"`
}

type SecretOutput struct {
	Value string `json:"value"`
	// Use pointer for error so if it's not provided null will be used
	Error *string `json:"error"`
}

func main() {
	in := Input{}
	err := json.NewDecoder(os.Stdin).Decode(&in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error occurred decoding input: %s", err)
		os.Exit(1)
	}

	// Append secret to all the secrets requested

	out := map[string]SecretOutput{}
	for _, s := range in.Secrets {
		out[s] = SecretOutput{
			Value: fmt.Sprintf("secret_%s", s),
		}
	}

	err = json.NewEncoder(os.Stdout).Encode(out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error occurred encoding output: %s", err)
		os.Exit(1)
	}
}
