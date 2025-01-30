// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cert provide useful functions to generate certificates
package cert

import (
	"bytes"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	configModel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultCertFileName represent the default IPC certificate root name (without .cert or .key)
const defaultCertFileName = "ipc_cert.pem"

// GetCertFilepath returns the path to the IPC cert file.
func GetCertFilepath(config configModel.Reader) string {
	if configPath := config.GetString("ipc_cert_file_path"); configPath != "" {
		return configPath
	}
	// Since customers who set the "auth_token_file_path" configuration likely prefer to avoid writing it next to the configuration file,
	// we should follow this behavior for the cert/key generation as well to minimize the risk of disrupting IPC functionality.
	if config.GetString("auth_token_file_path") != "" {
		dest := filepath.Join(filepath.Dir(config.GetString("auth_token_file_path")), defaultCertFileName)
		log.Warnf("IPC cert/key created or retrieved next to auth_token_file_path location: %v", dest)
		return dest
	}
	return filepath.Join(filepath.Dir(config.ConfigFileUsed()), defaultCertFileName)
}

// FetchAgentIPCCert return the IPC certificate and key from the path set in the configuration
// Requires that the config has been set up before calling
func FetchAgentIPCCert(config configModel.Reader) ([]byte, []byte, error) {
	return fetchAgentIPCCert(config, false)
}

// CreateOrFetchAgentIPCCert return the IPC certificate and key from the path set in the configuration or create if not present
// Requires that the config has been set up before calling
func CreateOrFetchAgentIPCCert(config configModel.Reader) ([]byte, []byte, error) {
	return fetchAgentIPCCert(config, true)
}

func fetchAgentIPCCert(config configModel.Reader, certCreationAllowed bool) ([]byte, []byte, error) {
	certPath := GetCertFilepath(config)

	// Create cert&key if it doesn't exist and if permitted by calling func
	if _, e := os.Stat(certPath); os.IsNotExist(e) && certCreationAllowed {
		// print the caller to identify what is calling this function
		if _, file, line, ok := runtime.Caller(2); ok {
			log.Infof("[%s:%d] Creating a new IPC certificate", file, line)
		}

		cert, key, err := generateCertKeyPair()

		if err != nil {
			return nil, nil, err
		}

		// Write the IPC cert/key in the FS (platform-specific)
		e = saveIPCCertKey(cert, key, certPath)
		if e != nil {
			return nil, nil, fmt.Errorf("error writing IPC cert/key file on fs: %s", e)
		}
		log.Infof("Saved a new  IPC certificate/key pair to %s", certPath)

		return cert, key, nil
	}

	// Read the IPC certAndKey/key
	certAndKey, e := os.ReadFile(certPath)
	if e != nil {
		return nil, nil, fmt.Errorf("unable to read authentication IPC cert/key files: %s", e.Error())
	}

	// Reading and decoding cert and key from file
	var block *pem.Block

	block, rest := pem.Decode(certAndKey)

	if block == nil || block.Type != "CERTIFICATE" {
		return nil, nil, log.Error("failed to decode PEM block containing certificate")
	}
	cert := pem.EncodeToMemory(block)

	block, _ = pem.Decode(rest)

	if block == nil || block.Type != "EC PRIVATE KEY" {
		return nil, nil, log.Error("failed to decode PEM block containing key")
	}

	key := pem.EncodeToMemory(block)

	return cert, key, nil
}

// writes IPC cert/key files to a file with the same permissions as datadog.yaml
func saveIPCCertKey(cert, key []byte, dest string) (err error) {
	log.Infof("Saving a new IPC certificate/key pair in %s", dest)

	perms, err := filesystem.NewPermission()
	if err != nil {
		return err
	}

	// Concatenating cert and key together
	certAndKey := bytes.Join([][]byte{cert, key}, []byte{})

	if err = os.WriteFile(dest, certAndKey, 0o600); err != nil {
		return err
	}

	if err := perms.RestrictAccessToUser(dest); err != nil {
		log.Errorf("Failed to set IPC cert permissions: %s", err)
		return err
	}

	log.Infof("Wrote IPC certificate/key pair in %s", dest)
	return nil
}
