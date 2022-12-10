// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets
// +build secrets

package secrets

import (
	"fmt"
	"strings"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultSecretBackendTimeout       = 5
	defaultSecretBackendOutputMaxSize = 1024 * 1024
)

var (
	secretCache map[string]string
	// list of handles and where they were found
	secretOrigin map[string]common.StringSet

	secretBackendCommand               string
	secretBackendArguments             []string
	secretBackendTimeout               = defaultSecretBackendTimeout
	secretBackendCommandAllowGroupExec bool
	secretBackendCommandSHA256         string
	configFileUsed                     string

	// SecretBackendOutputMaxSize defines max size of the JSON output from a secrets reader backend
	SecretBackendOutputMaxSize = defaultSecretBackendOutputMaxSize
)

func init() {
	secretCache = make(map[string]string)
	secretOrigin = make(map[string]common.StringSet)
}

// Init initializes the command and other options of the secrets package. Since
// this package is used by the 'config' package to decrypt itself we can't
// directly use it.
func Init(command string, arguments []string, timeout int, maxSize int, groupExecPerm bool, sha256 string, configFile string) {
	secretBackendCommand = command
	secretBackendArguments = arguments
	secretBackendTimeout = timeout
	SecretBackendOutputMaxSize = maxSize
	secretBackendCommandAllowGroupExec = groupExecPerm
	secretBackendCommandSHA256 = sha256
	configFileUsed = configFile
	if secretBackendCommandAllowGroupExec {
		log.Warnf("Agent configuration relax permissions constraint on the secret backend cmd, Group can read and exec")
	}
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
func Decrypt(data []byte, origin string) ([]byte, error) {
	if data == nil || secretBackendCommand == "" {
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
				// keep track of place where a handle was found
				secretOrigin[handle].Add(origin)
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
		secrets, err := secretFetcher(newHandles, origin)
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
func GetDebugInfo() (*SecretInfo, error) {
	if secretBackendCommand == "" {
		return nil, fmt.Errorf("No secret_backend_command set: secrets feature is not enabled")
	}
	info := &SecretInfo{
		ExecutablePath:       secretBackendCommand,
		ExecutablePathSHA256: secretBackendCommandSHA256,
	}
	info.populateRights()

	info.SecretsHandles = map[string][]string{}
	for handle, originNames := range secretOrigin {
		info.SecretsHandles[handle] = originNames.GetAll()
	}
	return info, nil
}
