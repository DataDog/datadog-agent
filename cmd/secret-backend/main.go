package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/rapdev-io/datadog-secret-backend/backend"
)

type InputPayload struct {
	Secrets []string `json:"secrets"`
	Version string   `json:"version"`
}

const appVersion = "0.1.3"

func init() {
	log.SetFormatter(&log.JSONFormatter{})
}

func printVersion() {
	fmt.Fprintf(os.Stdout, "%s: v%s\n\nRapDev (https://www.rapdev.io) (c) 2021\n",
		filepath.Base(os.Args[0]), appVersion)
	os.Exit(0)
}

func main() {
	program, _ := os.Executable()
	programPath := filepath.Dir(program)
	defaultConfigFile := filepath.Join(programPath, "datadog-secret-backend.yaml")

	version := flag.Bool("version", false,
		fmt.Sprintf("Displays version and information of %s", filepath.Base(program)),
	)
	configFile := flag.String("config", defaultConfigFile, "Path to backend configuration yaml")

	flag.Parse()

	if *version {
		printVersion()
	}

	input, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.WithError(err).Fatal("failed to read from input")
	}

	inputPayload := &InputPayload{}
	if err := json.Unmarshal(input, inputPayload); err != nil {
		log.WithError(err).Fatal("failed to unmarshal input")
	}

	backends := backend.NewBackends(configFile)
	secretOutputs := backends.GetSecretOutputs(inputPayload.Secrets)

	output, err := json.Marshal(secretOutputs)
	if err != nil {
		log.WithError(err).Fatal("failed to marshal output")
	}

	fmt.Printf(string(output))
}
