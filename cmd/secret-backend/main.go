package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

type InputPayload struct {
	Secrets []string `json:"secrets"`
	Version string   `json:"version"`
}

type SecretOutput struct {
	Value *string `json:"value"`
	Error *string `json:"error"`
}

const appVersion = "0.0.1"

func init() {
	log.SetFormatter(&log.JSONFormatter{})
}

func printVersion() {
	fmt.Fprintf(os.Stdout, "%s: v%s\n\nRapDev (https://www.rapdev.io) (c) 2021\n",
		filepath.Base(os.Args[0]), appVersion)
	os.Exit(0)
}

func main() {
	version := flag.Bool("version", false,
		fmt.Sprintf("displays version and information of %s", os.Args[0]),
	)
	configFile := flag.String("config", "secrets.yml", "path to configuration yaml")

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

	// Load SecretConfigurations
	secretConfigs := NewSecretConfigurations(configFile)

	secrets := NewSecrets()
	secretOutputs := make(map[string]SecretOutput)
	for _, s := range inputPayload.Secrets {
		segments := strings.SplitN(s, ":", 2)
		secretId := segments[0]
		secretKey := segments[1]

		if _, ok := secretConfigs[secretId]; !ok {
			log.WithField("secret", secretId).Error("undefined secret")
			secrets.Secrets[secretId] = &ErrorSecret{
				SecretId: secretId,
				Error:    fmt.Errorf("secret not defined in configuration"),
			}
			secretOutputs[s] = secrets.Secrets[secretId].GetSecretOutput(secretKey)
		} else {
			secrets.InitSecret(secretConfigs[secretId], secretId)
			secretOutputs[s] = secrets.Secrets[secretId].GetSecretOutput(secretKey)
		}

	}

	output, err := json.Marshal(secretOutputs)
	if err != nil {
		log.WithError(err).Fatal("failed to marshal output")
	}

	fmt.Printf(string(output))
}
