// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	"fmt"
	"os"
	"strings"
)

var _ valueStore = &envValueStore{}

var envVariablesByStoreKey = map[StoreKey]string{
	APIKey:                       "E2E_API_KEY",
	APPKey:                       "E2E_APP_KEY",
	Environments:                 "E2E_ENVIRONMENTS",
	ExtraResourcesTags:           "E2E_EXTRA_RESOURCES_TAGS",
	KeyPairName:                  "E2E_KEY_PAIR_NAME",
	PrivateKeyPassword:           "E2E_PRIVATE_KEY_PASSWORD",
	PrivateKeyPath:               "E2E_PRIVATE_KEY_PATH",
	Profile:                      "E2E_PROFILE",
	PublicKeyPath:                "E2E_PUBLIC_KEY_PATH",
	PulumiPassword:               "E2E_PULUMI_PASSWORD",
	SkipDeleteOnFailure:          "E2E_SKIP_DELETE_ON_FAILURE",
	StackParameters:              "E2E_STACK_PARAMS",
	PipelineID:                   "E2E_PIPELINE_ID",
	CommitSHA:                    "E2E_COMMIT_SHA",
	VerifyCodeSignature:          "E2E_VERIFY_CODE_SIGNATURE",
	OutputDir:                    "E2E_OUTPUT_DIR",
	PulumiLogLevel:               "E2E_PULUMI_LOG_LEVEL",
	PulumiLogToStdErr:            "E2E_PULUMI_LOG_TO_STDERR",
	PulumiVerboseProgressStreams: "E2E_PULUMI_VERBOSE_PROGRESS_STREAMS",
	DevMode:                      "E2E_DEV_MODE",
}

type envValueStore struct {
	prefix string
}

// NewEnvStore creates a new store based on environment variables
func NewEnvStore(prefix string) Store {
	return newStore(newEnvValueStore(prefix))
}

func newEnvValueStore(prefix string) envValueStore {
	return envValueStore{
		prefix: prefix,
	}
}

// Get returns parameter value.
// For env Store, the key is upper cased and added to prefix
func (s envValueStore) get(key StoreKey) (string, error) {
	envValueStoreKey := envVariablesByStoreKey[key]
	if envValueStoreKey == "" {
		fmt.Printf("key [%s] not found in envValueStoreKey, converting to `strings.ToUpper(E2E_<key>)`\n", key)
		envValueStoreKey = strings.ToUpper(s.prefix + string(key))
	}
	val, found := os.LookupEnv(strings.ToUpper(envValueStoreKey))
	if !found {
		return "", ParameterNotFoundError{key: key}
	}

	return val, nil
}
