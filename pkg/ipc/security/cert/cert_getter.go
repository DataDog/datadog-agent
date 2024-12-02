// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cert provide useful functions to generate certificates
package cert

import (
	"fmt"
	"os"
	"runtime"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	certLock sync.RWMutex
)

// FetchAgentIPCCert gets the authentication token from the auth token file & creates one if it doesn't exist
// Requires that the config has been set up before calling
func FetchAgentIPCCert(destDir string) ([]byte, []byte, error) {
	certLock.Lock()
	defer certLock.Unlock()
	return fetchAgentIPCCert(destDir, false)
}

// CreateOrFetchAgentIPCCert gets the authentication token from the auth token file & creates one if it doesn't exist
// Requires that the config has been set up before calling
func CreateOrFetchAgentIPCCert(destDir string) ([]byte, []byte, error) {
	certLock.Lock()
	defer certLock.Unlock()
	return fetchAgentIPCCert(destDir, true)
}

func fetchAgentIPCCert(certPath string, certCreationAllowed bool) ([]byte, []byte, error) {
	// Create a new token if it doesn't exist and if permitted by calling func
	if _, e := os.Stat(certPath + ".cert"); os.IsNotExist(e) && certCreationAllowed {
		// print the caller to identify what is calling this function
		if _, file, line, ok := runtime.Caller(2); ok {
			log.Infof("[%s:%d] Creating a new IPC certificate", file, line)
		}

		hosts := []string{"127.0.0.1", "localhost", "::1"}
		// hosts = append(hosts, additionalHostIdentities...)
		cert, key, err := generateCertKeyPair(hosts, 2048)

		if err != nil {
			return nil, nil, err
		}

		// Write the auth token to the auth token file (platform-specific)
		e = saveIPCCertKey(cert, key, certPath)
		if e != nil {
			return nil, nil, fmt.Errorf("error writing authentication token file on fs: %s", e)
		}
		log.Infof("Saved a new  IPC certificate/key pair to %s", certPath)

		return cert, key, nil
	}

	// Read the token
	cert, e := os.ReadFile(certPath + ".cert")
	if e != nil {
		return nil, nil, fmt.Errorf("unable to read authentication token file: %s", e.Error())
	}
	key, e := os.ReadFile(certPath + ".key")
	if e != nil {
		return nil, nil, fmt.Errorf("unable to read authentication token file: %s", e.Error())
	}

	return cert, key, nil
}

// writes auth token(s) to a file with the same permissions as datadog.yaml
func saveIPCCertKey(cert, key []byte, dest string) error {
	log.Infof("Saving a new IPC certificate/key pair in %s", dest)
	if err := os.WriteFile(dest+".cert", cert, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(dest+".key", key, 0o600); err != nil {
		return err
	}

	perms, err := filesystem.NewPermission()
	if err != nil {
		return err
	}

	if err := perms.RestrictAccessToUser(dest + ".cert"); err != nil {
		log.Errorf("Failed to write auth token acl %s", err)
		return err
	}

	if err := perms.RestrictAccessToUser(dest + ".key"); err != nil {
		log.Errorf("Failed to write auth token acl %s", err)
		return err
	}

	log.Infof("Wrote IPC certificate/key pair in %s", dest)
	return nil
}
