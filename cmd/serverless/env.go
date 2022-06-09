package main

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// decryptFunc is a function that takes in a value and retrieves it from
// the appropriate AWS service. KMS, SM, etc.
type decryptFunc func(string) (string, error)

func getSecretEnvVars(envVars []string, kmsFunc decryptFunc, smFunc decryptFunc) map[string]string {
	decryptedEnvVars := make(map[string]string)
	for _, envVar := range envVars {
		// TODO: Replace with strings.Cut in Go 1.18
		tokens := strings.SplitN(envVar, "=", 2)
		if len(tokens) != 2 {
			continue
		}
		envKey := tokens[0]
		envVal := tokens[1]
		if strings.HasSuffix(envKey, kmsKeySuffix) {
			log.Debugf("Decrypting %v", envVar)
			secretVal, err := kmsFunc(envVal)
			if err != nil {
				log.Debugf("Couldn't read API key from KMS: %v", err)
				continue
			}
			decryptedEnvVars[strings.TrimSuffix(envVar, kmsKeySuffix)] = secretVal
		}
		if strings.HasSuffix(envKey, secretArnSuffix) {
			log.Debugf("Retrieving %v from secrets manager", envVar)
			secretVal, err := smFunc(envVal)
			if err != nil {
				log.Debugf("Couldn't read API key from KMS: %v", err)
				continue
			}
			decryptedEnvVars[strings.TrimSuffix(envVar, secretArnSuffix)] = secretVal
		}
	}
	return decryptedEnvVars
}
