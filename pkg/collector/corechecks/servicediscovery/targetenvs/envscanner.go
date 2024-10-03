// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package targetenvs

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/security/ptracer"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	// maxSizeEnvsMap - maximum number of returned environment variables
	maxSizeEnvsMap = 400
)

// EnvScanner reads the environment variables from /proc/<pid>/environ file.
// It collects only those variables that match the target map if the map is not empty,
// otherwise collect all environment variables.
type EnvScanner struct {
	file    *os.File                     // open pointer to environment variables file
	scanner *ptracer.TextScannerIterator // iterator to read strings from text file
	targets map[uint64]string            // map of environment variables of interest
	envs    map[string]string            // collected environment variables
}

// hashBytes return hash value of a bytes array using FNV-1a hash function
func hashBytes(b []byte) uint64 {
	h := fnv.New64a()
	h.Write(b)
	return h.Sum64()
}

// newEnvScanner returns a new [EnvScanner] to read from path.
func newEnvScanner(proc *process.Process) (*EnvScanner, error) {
	envPath := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "environ")
	file, err := os.Open(envPath)
	if err != nil {
		return nil, err
	}

	return &EnvScanner{
		file:    file,
		scanner: ptracer.NewTextScannerIterator(file),
		targets: targetsMap,
		envs:    make(map[string]string, len(targetsMap)),
	}, nil
}

// finish closes an open file.
func (es *EnvScanner) finish() {
	if es.file != nil {
		es.file.Close()
	}
}

// add adds env. variable to the map of environment variables.
func (es *EnvScanner) add() error {
	if len(es.envs) == maxSizeEnvsMap {
		return fmt.Errorf("read proc env can't add more than max (%d) environment variables", maxSizeEnvsMap)
	}
	b := es.scanner.Bytes()
	eq := bytes.IndexByte(b, '=')
	if eq == -1 {
		// ignore invalid env. variable
		return nil
	}

	h := hashBytes(b[:eq])
	_, exists := es.targets[h]
	if exists {
		name := string(b[:eq])
		es.envs[name] = string(b[eq+1:])
	}

	return nil
}

// GetEnvs searches the environment variables of interest in the /proc/<pid>/environ file.
func GetEnvs(proc *process.Process) (map[string]string, error) {
	es, err := newEnvScanner(proc)
	if err != nil {
		return nil, err
	}
	defer es.finish()

	for es.scanner.Next() {
		err := es.add()
		if err != nil {
			return es.envs, err
		}
	}
	if err := es.scanner.Err(); err != nil {
		return es.envs, err
	}
	return es.envs, nil
}
