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
	AWSPrivateKeyPassword:        "E2E_AWS_PRIVATE_KEY_PASSWORD",
	AWSPrivateKeyPath:            "E2E_AWS_PRIVATE_KEY_PATH",
	Profile:                      "E2E_PROFILE",
	AWSPublicKeyPath:             "E2E_AWS_PUBLIC_KEY_PATH",
	AzurePrivateKeyPath:          "E2E_AZURE_PRIVATE_KEY_PATH",
	AzurePublicKeyPath:           "E2E_AZURE_PUBLIC_KEY_PATH",
	AzurePrivateKeyPassword:      "E2E_AZURE_PRIVATE_KEY_PASSWORD",
	GCPPrivateKeyPath:            "E2E_GCP_PRIVATE_KEY_PATH",
	GCPPublicKeyPath:             "E2E_GCP_PUBLIC_KEY_PATH",
	GCPPrivateKeyPassword:        "E2E_GCP_PRIVATE_KEY_PASSWORD",
	LocalPublicKeyPath:           "E2E_LOCAL_PUBLIC_KEY_PATH",
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
	InitOnly:                     "E2E_INIT_ONLY",
	TeardownOnly:                 "E2E_TEARDOWN_ONLY",
	MajorVersion:                 "E2E_MAJOR_VERSION",
	PreInitialized:               "E2E_PRE_INITIALIZED",
	FIPS:                         "E2E_FIPS",
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
