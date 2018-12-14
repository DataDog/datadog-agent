// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package secrets

import (
	"fmt"
	"io"
	"strings"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	secretCache map[string]string

	secretBackendCommand       string
	secretBackendArguments     []string
	secretBackendTimeout       = 5
	secretBackendOutputMaxSize = 1024
)

func init() {
	secretCache = make(map[string]string)
}

// Init initializes the command and other options of the secrets package. Since
// this package is used by the 'config' package to decrypt itself we can't
// directly use it.
func Init(command string, arguments []string, timeout int, maxSize int) {
	secretBackendCommand = command
	secretBackendArguments = arguments
	secretBackendTimeout = timeout
	secretBackendOutputMaxSize = maxSize
}

type walkerCallback func(string) (string, error)

func walkSlice(data []interface{}, callback walkerCallback) error {
	for idx, k := range data {
		switch v := k.(type) {
		case string:
			newValue, err := callback(v)
			if err != nil {
				return err
			}
			data[idx] = newValue
		case map[interface{}]interface{}:
			if err := walkHash(v, callback); err != nil {
				return err
			}
		case []interface{}:
			if err := walkSlice(v, callback); err != nil {
				return err
			}
		}
	}
	return nil
}

func walkHash(data map[interface{}]interface{}, callback walkerCallback) error {
	for k := range data {
		switch v := data[k].(type) {
		case string:
			newValue, err := callback(v)
			if err != nil {
				return err
			}
			data[k] = newValue
		case map[interface{}]interface{}:
			if err := walkHash(v, callback); err != nil {
				return err
			}
		case []interface{}:
			if err := walkSlice(v, callback); err != nil {
				return err
			}
		}
	}
	return nil
}

// walk will go through loaded yaml and call callback on every strings allowing
// the callback to overwrite the string value
func walk(data *interface{}, callback walkerCallback) error {
	switch v := (*data).(type) {
	case string:
		newValue, err := callback(v)
		if err != nil {
			return err
		}
		*data = newValue
	case map[interface{}]interface{}:
		return walkHash(v, callback)
	case []interface{}:
		return walkSlice(v, callback)
	}
	return nil
}

func isEnc(str string) (bool, string) {
	// trimming space and tabs
	str = strings.Trim(str, " 	")
	if strings.HasPrefix(str, "ENC[") && strings.HasSuffix(str, "]") {
		return true, str[4 : len(str)-1]
	}
	return false, ""
}

// testing purpose
var secretFetcher = fetchSecret

// Decrypt replaces all encrypted secrets in data by executing
// "secret_backend_command" once if all secrets aren't present in the cache.
func Decrypt(data []byte) ([]byte, error) {
	if data == nil || secretBackendCommand == "" {
		log.Debugf("No data to decrypt or no secretBackendCommand set: skipping")
		return data, nil
	}

	var config interface{}
	err := yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("could not Unmarshal config: %s", err)
	}

	// First we collect all new handles in the config
	newHandles := []string{}
	haveSecret := false
	err = walk(&config, func(str string) (string, error) {
		if ok, handle := isEnc(str); ok {
			haveSecret = true
			// Check if we already know this secret
			if secret, ok := secretCache[handle]; ok {
				log.Debugf("Secret '%s' was retrieved from cache", handle)
				return secret, nil
			}
			newHandles = append(newHandles, handle)
		}
		return str, nil
	})
	if err != nil {
		return nil, err
	}

	// the configuration does not contain any secrets
	if !haveSecret {
		return data, nil
	}

	// check if any new secrets need to be fetch
	if len(newHandles) != 0 {
		secrets, err := secretFetcher(newHandles)
		if err != nil {
			return nil, err
		}

		// Replace all new encrypted secrets in the config
		err = walk(&config, func(str string) (string, error) {
			if ok, handle := isEnc(str); ok {
				if secret, ok := secrets[handle]; ok {
					log.Debugf("Secret '%s' was retrieved from executable", handle)
					return secret, nil
				}
				// This should never happen since fetchSecret will return an error
				// if not every handles have been fetched.
				return str, fmt.Errorf("unknown secret '%s'", handle)
			}
			return str, nil
		})
		if err != nil {
			return nil, err
		}
	}

	finalConfig, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("could not Marshal config after replacing encrypted secrets: %s", err)
	}
	return finalConfig, nil
}

// GetDebugInfo exposes debug informations about secrets to be included in a flare
func GetDebugInfo(w io.Writer) {
	if secretBackendCommand == "" {
		fmt.Fprintln(w, "No secret_backend_command set: secrets feature is not enabled")
		return
	}

	listRights(secretBackendCommand, w)

	fmt.Fprintf(w, "=== Secrets stats ===\n")
	fmt.Fprintf(w, "Number of secrets decrypted: %d\n", len(secretCache))
	fmt.Fprintln(w, "secrets Handle decrypted:")
	for handle := range secretCache {
		fmt.Fprintf(w, "- %s\n", handle)
	}
}
