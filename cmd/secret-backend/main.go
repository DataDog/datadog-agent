package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/rapdev-io/datadog-secret-backend/backend"
)

type InputPayload struct {
	Secrets []string `json:"secrets"`
	Version string   `json:"version"`
}

const appVersion = "0.1.11"

var Log zerolog.Logger

func init() {
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}
	output := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
		FormatLevel: func(i interface{}) string {
			return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
		},
	}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()
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
		log.Fatal().Err(err).Msg("failed to read from stdin")
	}

	inputPayload := &InputPayload{}
	if err := json.Unmarshal(input, inputPayload); err != nil {
		log.Fatal().Err(err).Msg("failed to unmarshal input")
	}

	backends := backend.NewBackends(configFile)
	secretOutputs := backends.GetSecretOutputs(inputPayload.Secrets)

	output, err := json.Marshal(secretOutputs)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to marshal output")
	}

	fmt.Printf(string(output))
}
