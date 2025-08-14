// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"fmt"
	"strings"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
)

// KubeSecretGetter is a function that fetches a secret from k8s
type KubeSecretGetter func(string, string) (map[string][]byte, error)

// ReadKubernetesSecret reads a secrets store in k8s
func ReadKubernetesSecret(readSecretFromKubeClient KubeSecretGetter, path string) secrets.SecretVal {
	splitName := strings.Split(path, "/")

	if len(splitName) != 3 {
		return secrets.SecretVal{ErrorMsg: "invalid format. Use: \"namespace/name/key\""}
	}

	namespace, name, key := splitName[0], splitName[1], splitName[2]

	secret, err := readSecretFromKubeClient(namespace, name)
	if err != nil {
		return secrets.SecretVal{ErrorMsg: err.Error()}
	}

	value, ok := secret[key]
	if !ok {
		return secrets.SecretVal{ErrorMsg: fmt.Sprintf("key %s not found in secret %s/%s", key, namespace, name)}
	}

	return secrets.SecretVal{Value: string(value)}
}
