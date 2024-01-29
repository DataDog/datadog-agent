// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package oidresolverimpl implements the OID Resolver component.
package oidresolverimpl

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newResolver),
)

const ddTrapDBFileNamePrefix string = "dd_traps_db"

var nodesOIDThatShouldNeverMatch = []string{
	"1.3.6.1.4.1", // "iso.org.dod.internet.private.enterprises". This OID and all its parents are known "intermediate" nodes
	"1.3.6.1.4",   // "iso.org.dod.internet.private"
	"1.3.6.1",     // "iso.org.dod.internet"
	"1.3.6",       // "iso.org.dod"
	"1.3",         // "iso.org"
	"1",           // "iso"
}

type unmarshaller func(data []byte, v interface{}) error

// multiFilesOIDResolver is an OIDResolver implementation that can be configured with multiple input files.
// Trap OIDs conflicts are resolved using the name of the source file in alphabetical order and by giving
// the less priority to Datadog's own database shipped with the agent.
// Variable OIDs conflicts are fully resolved by also looking at the trap OID. A given trap OID only
// exist in a single file (after the previous conflict resolution), meaning that we get the variable
// metadata from that same file.
type multiFilesOIDResolver struct {
	traps  oidresolver.TrapSpec
	logger log.Component
}

func newResolver(conf config.Component, logger log.Component) (oidresolver.Component, error) {
	return newMultiFilesOIDResolver(conf.GetString("confd_path"), logger)
}

// newMultiFilesOIDResolver creates a new MultiFilesOIDResolver instance by loading json or yaml files
// (optionnally gzipped) located in the directory snmp.d/traps_db/
func newMultiFilesOIDResolver(confdPath string, logger log.Component) (*multiFilesOIDResolver, error) {
	oidResolver := &multiFilesOIDResolver{
		traps:  make(oidresolver.TrapSpec),
		logger: logger,
	}
	trapsDBRoot := filepath.Join(confdPath, "snmp.d", "traps_db")
	files, err := os.ReadDir(trapsDBRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir `%s`: %w", trapsDBRoot, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("dir `%s` does not contain any trap db file", trapsDBRoot)
	}
	fileNames := getSortedFileNames(files, logger)
	for _, fileName := range fileNames {
		err := oidResolver.updateFromFile(filepath.Join(trapsDBRoot, fileName))
		if err != nil {
			logger.Warnf("unable to load trap db file %s: %s", fileName, err)
		}
	}
	return oidResolver, nil
}

// GetTrapMetadata returns TrapMetadata for a given trapOID
func (or *multiFilesOIDResolver) GetTrapMetadata(trapOID string) (oidresolver.TrapMetadata, error) {
	trapOID = strings.TrimSuffix(oidresolver.NormalizeOID(trapOID), ".0")
	trapData, ok := or.traps[trapOID]
	if !ok {
		return oidresolver.TrapMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOID)
	}
	return trapData, nil
}

// GetVariableMetadata returns VariableMetadata for a given variableOID and trapOID.
// The trapOID should not be needed in theory but the Datadog Agent allows to define multiple variable names for the
// same OID as long as they are defined in different file. The trapOID is used to differentiate between these files.
func (or *multiFilesOIDResolver) GetVariableMetadata(trapOID string, varOID string) (oidresolver.VariableMetadata, error) {
	trapOID = strings.TrimSuffix(oidresolver.NormalizeOID(trapOID), ".0")
	varOID = strings.TrimSuffix(oidresolver.NormalizeOID(varOID), ".0")
	trapData, ok := or.traps[trapOID]
	if !ok {
		return oidresolver.VariableMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOID)
	}

	recreatedVarOID := varOID
	for {
		varData, ok := trapData.VariableSpecPtr[recreatedVarOID]
		if ok {
			if varData.IsIntermediateNode {
				// Found a known Node while climibing up the tree, no chance of finding a match higher
				return oidresolver.VariableMetadata{}, fmt.Errorf("variable OID %s is not defined", varOID)
			}
			return varData, nil

		}
		// No match for the current varOID, climb up the tree and retry
		lastDot := strings.LastIndex(recreatedVarOID, ".")
		if lastDot == -1 {
			break
		}
		recreatedVarOID = varOID[:lastDot]
	}
	return oidresolver.VariableMetadata{}, fmt.Errorf("variable OID %s is not defined", varOID)
}

func getSortedFileNames(files []fs.DirEntry, logger log.Component) []string {
	if len(files) == 0 {
		return []string{}
	}
	// There should usually be one file provided by Datadog and zero or more provided by the user
	userProvidedFileNames := make([]string, 0, len(files)-1)
	// Using a slice for error-proofing but there will usually be only one dd provided file.
	ddProvidedFileNames := make([]string, 0, 1)
	for _, file := range files {
		if file.IsDir() {
			logger.Debugf("not loading traps data from path %s: file is directory", file.Name())
			continue
		}
		fileName := file.Name()
		if strings.HasPrefix(fileName, ddTrapDBFileNamePrefix) {
			ddProvidedFileNames = append(ddProvidedFileNames, fileName)
		} else {
			userProvidedFileNames = append(userProvidedFileNames, file.Name())
		}
	}

	sort.Slice(userProvidedFileNames, func(i, j int) bool {
		return strings.ToLower(userProvidedFileNames[i]) < strings.ToLower(userProvidedFileNames[j])
	})
	sort.Slice(ddProvidedFileNames, func(i, j int) bool {
		return strings.ToLower(ddProvidedFileNames[i]) < strings.ToLower(ddProvidedFileNames[j])
	})

	return append(ddProvidedFileNames, userProvidedFileNames...)
}

func (or *multiFilesOIDResolver) updateFromFile(filePath string) error {
	var fileReader io.ReadCloser
	fileReader, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer fileReader.Close()
	if strings.HasSuffix(filePath, ".gz") {
		filePath = strings.TrimSuffix(filePath, ".gz")
		uncompressor, err := gzip.NewReader(fileReader)
		if err != nil {
			return fmt.Errorf("unable to uncompress gzip file %s", filePath)
		}
		defer uncompressor.Close()
		fileReader = uncompressor
	}
	var unmarshalMethod unmarshaller = yaml.Unmarshal
	if strings.HasSuffix(filePath, ".json") {
		unmarshalMethod = json.Unmarshal
	}
	return or.updateFromReader(fileReader, unmarshalMethod)
}

func (or *multiFilesOIDResolver) updateFromReader(reader io.Reader, unmarshalMethod unmarshaller) error {
	fileContent, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	var trapData oidresolver.TrapDBFileContent
	err = unmarshalMethod(fileContent, &trapData)
	if err != nil {
		return err
	}

	or.updateResolverWithData(trapData)
	return nil
}

func (or *multiFilesOIDResolver) updateResolverWithData(trapDB oidresolver.TrapDBFileContent) {
	definedVariables := oidresolver.VariableSpec{}

	allOIDs := make([]string, 0, len(trapDB.Variables))
	for variableOID := range trapDB.Variables {
		if !oidresolver.IsValidOID(variableOID) {
			or.logger.Warnf("trap variable OID %s does not look like a valid OID", variableOID)
			continue
		}
		allOIDs = append(allOIDs, oidresolver.NormalizeOID(variableOID))
	}

	// "Fast" algorithm used to mark OID that act both as a variable and as a parent of other variable
	// with 'isNode: true'. i.e if an OID <FOO>.<BAR> exists in the trapsDB but <FOO> also exists in the trapsDB
	// then <FOO> acts as a 'Node' of the OID tree and should not be considered a match for resolving variables.
	// In this fast algorithm the list is sorted then each OID is compared with its successor. It the successor starts
	// with the current OID + a dot, then the current OID is a Node. 'Dots' are before digits in the lexicographic order.
	// Note that in practice, OIDs that act both as Node and Leaf of the OID tree is extremely rare and is not expected
	// in normal circumstamces. Thing is they sometimes exist.
	sort.Strings(allOIDs)
	for idx, variableOID := range allOIDs {
		isIntermediateNode := false
		if idx+1 < len(allOIDs) {
			nextOID := allOIDs[idx+1]
			isIntermediateNode = strings.HasPrefix(nextOID, variableOID+".")
		}

		variableData := trapDB.Variables[variableOID]
		variableData.IsIntermediateNode = isIntermediateNode
		definedVariables[variableOID] = variableData
	}

	for _, nodeOID := range nodesOIDThatShouldNeverMatch {
		definedVariables[nodeOID] = oidresolver.VariableMetadata{Name: "unknown", IsIntermediateNode: true}
	}

	for trapOID, trapData := range trapDB.Traps {
		if !oidresolver.IsValidOID(trapOID) {
			or.logger.Errorf("trap OID %s does not look like a valid OID", trapOID)
			continue
		}
		trapOID := oidresolver.NormalizeOID(trapOID)
		if _, trapConflict := or.traps[trapOID]; trapConflict {
			or.logger.Debugf("a trap with OID %s is defined in multiple traps db files", trapOID)
		}
		or.traps[trapOID] = oidresolver.TrapMetadata{
			Name:            trapData.Name,
			Description:     trapData.Description,
			MIBName:         trapData.MIBName,
			VariableSpecPtr: definedVariables,
		}
	}
}
