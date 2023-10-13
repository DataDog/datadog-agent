// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package oidresolver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type MockedDirEntry struct {
	name  string
	isDir bool
}

func (m MockedDirEntry) Info() (fs.FileInfo, error) {
	return nil, nil
}

func (m MockedDirEntry) IsDir() bool {
	return m.isDir
}

func (m MockedDirEntry) Name() string {
	return m.name
}

func (m MockedDirEntry) Type() fs.FileMode {
	return 0
}
func TestDecoding(t *testing.T) {
	trapDBFile := &TrapDBFileContent{
		Traps: TrapSpec{
			"foo": TrapMetadata{
				Name:    "xx",
				MIBName: "yy",
			},
		},
		Variables: variableSpec{
			"bar": VariableMetadata{
				Name:        "yy",
				Description: "dummy description",
				Enumeration: map[int]string{2: "test"},
				Bits:        map[int]string{3: "test3"},
			},
		},
	}
	data, err := json.Marshal(trapDBFile)
	require.NoError(t, err)
	require.Equal(t, []byte("{\"traps\":{\"foo\":{\"name\":\"xx\",\"mib\":\"yy\",\"descr\":\"\"}},\"vars\":{\"bar\":{\"name\":\"yy\",\"descr\":\"dummy description\",\"enum\":{\"2\":\"test\"},\"bits\":{\"3\":\"test3\"}}}}"), data)
	err = json.Unmarshal([]byte("{\"traps\": {\"1.2\": {\"name\": \"dd\"}}}"), &trapDBFile)
	require.NoError(t, err)
}

func TestSortFiles(t *testing.T) {
	logger := fxutil.Test[log.Component](t, log.MockModule)
	files := []fs.DirEntry{
		MockedDirEntry{name: "totoro", isDir: false},
		MockedDirEntry{name: "porco", isDir: false},
		MockedDirEntry{name: "Nausicaa", isDir: false},
		MockedDirEntry{name: "kiki", isDir: false},
		MockedDirEntry{name: "mononoke", isDir: false},
		MockedDirEntry{name: "ponyo", isDir: false},
		MockedDirEntry{name: "chihiro", isDir: false},
		MockedDirEntry{name: "directory", isDir: true},
		MockedDirEntry{name: "dd_traps_db.json.gz", isDir: true},
		MockedDirEntry{name: "dd_traps_db.json", isDir: true},
		MockedDirEntry{name: "dd_traps_db.yaml.gz", isDir: true},
		MockedDirEntry{name: "dd_traps_db.yaml", isDir: true},
		MockedDirEntry{name: "dd_traps_db.json.gz", isDir: false},
		MockedDirEntry{name: "dd_traps_db.json", isDir: false},
		MockedDirEntry{name: "dd_traps_db.yaml.gz", isDir: false},
		MockedDirEntry{name: "dd_traps_db.yaml", isDir: false},
	}
	sortedFiles := getSortedFileNames(files, logger)
	require.EqualValues(t,
		[]string{
			"dd_traps_db.json",
			"dd_traps_db.json.gz",
			"dd_traps_db.yaml",
			"dd_traps_db.yaml.gz",
			"chihiro",
			"kiki",
			"mononoke",
			"Nausicaa",
			"ponyo",
			"porco",
			"totoro",
		},
		sortedFiles,
	)
}

func TestResolverWithNonStandardOIDs(t *testing.T) {
	logger := fxutil.Test[log.Component](t, log.MockModule)
	resolver := &MultiFilesOIDResolver{traps: make(TrapSpec), logger: logger}
	trapData := TrapDBFileContent{
		Traps: TrapSpec{"1.3.6.1.4.1.8072.2.3.0.1": TrapMetadata{Name: "netSnmpExampleHeartbeat", MIBName: "NET-SNMP-EXAMPLES-MIB"}},
		Variables: variableSpec{
			"1.3.6.1.4.1.8072.2.3.2.1": VariableMetadata{
				Name: "netSnmpExampleHeartbeatRate",
			},
		},
	}
	updateResolverWithIntermediateJSONReader(t, resolver, trapData)

	data, err := resolver.GetTrapMetadata(".1.3.6.1.4.1.8072.2.3.0.1")
	require.NoError(t, err)
	require.Equal(t, "netSnmpExampleHeartbeat", data.Name)
	require.Equal(t, "NET-SNMP-EXAMPLES-MIB", data.MIBName)

	data, err = resolver.GetTrapMetadata("1.3.6.1.4.1.8072.2.3.0.1.0")
	require.NoError(t, err)
	require.Equal(t, "netSnmpExampleHeartbeat", data.Name)
	require.Equal(t, "NET-SNMP-EXAMPLES-MIB", data.MIBName)

	data, err = resolver.GetTrapMetadata(".1.3.6.1.4.1.8072.2.3.0.1.0")
	require.NoError(t, err)
	require.Equal(t, "netSnmpExampleHeartbeat", data.Name)
	require.Equal(t, "NET-SNMP-EXAMPLES-MIB", data.MIBName)

}
func TestResolverWithConflictingTrapOID(t *testing.T) {
	logger := fxutil.Test[log.Component](t, log.MockModule)
	resolver := &MultiFilesOIDResolver{traps: make(TrapSpec), logger: logger}
	trapDataA := TrapDBFileContent{
		Traps: TrapSpec{"1.3.6.1.4.1.8072.2.3.0.1": TrapMetadata{Name: "foo", MIBName: "FOO-MIB"}},
	}
	trapDataB := TrapDBFileContent{
		Traps: TrapSpec{"1.3.6.1.4.1.8072.2.3.0.1": TrapMetadata{Name: "bar", MIBName: "BAR-MIB"}},
	}
	updateResolverWithIntermediateJSONReader(t, resolver, trapDataA)
	updateResolverWithIntermediateYAMLReader(t, resolver, trapDataB)
	data, err := resolver.GetTrapMetadata("1.3.6.1.4.1.8072.2.3.0.1")
	require.NoError(t, err)
	require.Equal(t, "bar", data.Name)
	require.Equal(t, "BAR-MIB", data.MIBName)
}

func TestResolverWithConflictingVariables(t *testing.T) {
	logger := fxutil.Test[log.Component](t, log.MockModule)
	resolver := &MultiFilesOIDResolver{traps: make(TrapSpec), logger: logger}
	trapDataA := TrapDBFileContent{
		Traps: TrapSpec{"1.3.6.1.4.1.8072.2.3.0.1": TrapMetadata{}},
		Variables: variableSpec{
			"1.3.6.1.4.1.8072.2.3.2.1": VariableMetadata{
				Name: "netSnmpExampleHeartbeatRate",
			},
		},
	}
	trapDataB := TrapDBFileContent{
		Traps: TrapSpec{"1.3.6.1.4.1.8072.2.3.0.2": TrapMetadata{}},
		Variables: variableSpec{
			"1.3.6.1.4.1.8072.2.3.2.1": VariableMetadata{
				Name: "netSnmpExampleHeartbeatRate2",
			},
		},
	}
	updateResolverWithIntermediateJSONReader(t, resolver, trapDataA)
	updateResolverWithIntermediateYAMLReader(t, resolver, trapDataB)
	data, err := resolver.GetVariableMetadata("1.3.6.1.4.1.8072.2.3.0.1", "1.3.6.1.4.1.8072.2.3.2.1")
	require.NoError(t, err)
	require.Equal(t, "netSnmpExampleHeartbeatRate", data.Name)

	// Same variable OID, different trap OID
	data, err = resolver.GetVariableMetadata("1.3.6.1.4.1.8072.2.3.0.2", "1.3.6.1.4.1.8072.2.3.2.1")
	require.NoError(t, err)
	require.Equal(t, "netSnmpExampleHeartbeatRate2", data.Name)
}

func TestResolverWithSuffixedVariable(t *testing.T) {
	logger := fxutil.Test[log.Component](t, log.MockModule)
	resolver := &MultiFilesOIDResolver{traps: make(TrapSpec), logger: logger}
	updateResolverWithIntermediateJSONReader(t, resolver, dummyTrapDB)

	data, err := resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.2.1.2.2.1.1")
	require.NoError(t, err)
	require.Equal(t, "ifIndex", data.Name)

	data, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.2.1.2.2.1.1.0")
	require.NoError(t, err)
	require.Equal(t, "ifIndex", data.Name)

	data, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.2.1.2.2.1.1.123436")
	require.NoError(t, err)
	require.Equal(t, "ifIndex", data.Name)

	data, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.2.1.2.2.1.1.9.9.9.9.9.9.9.9.9.99999.9")
	require.NoError(t, err)
	require.Equal(t, "ifIndex", data.Name)
}

func TestResolverWithSuffixedVariableAndNodeConflict(t *testing.T) {
	logger := fxutil.Test[log.Component](t, log.MockModule)
	resolver := &MultiFilesOIDResolver{traps: make(TrapSpec), logger: logger}
	trapDB := TrapDBFileContent{
		Traps: TrapSpec{
			"1.3.6.1.6.3.1.1.5.4": TrapMetadata{Name: "linkUp", MIBName: "IF-MIB"},
		},
		Variables: variableSpec{
			"1.3.6.1.2.1.2.2":     VariableMetadata{Name: "NodeConflict"},
			"1.3.6.1.2.1.2.2.1.1": VariableMetadata{Name: "ifIndex"},
			"1.3.6.1.2.1.2.3":     VariableMetadata{Name: "NotAConflict"},
		},
	}
	updateResolverWithIntermediateJSONReader(t, resolver, trapDB)
	data, err := resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.2.1.2.2.1.1")
	require.NoError(t, err)
	require.Equal(t, "ifIndex", data.Name)

	data, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.2.1.2.2.1.1.0")
	require.NoError(t, err)
	require.Equal(t, "ifIndex", data.Name)

	data, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.2.1.2.2.1.1.123436")
	require.NoError(t, err)
	require.Equal(t, "ifIndex", data.Name)

	data, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.2.1.2.2.1.1.9.9.9.9.9.9.9.9.9.99999.9")
	require.NoError(t, err)
	require.Equal(t, "ifIndex", data.Name)

	data, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.2.1.2.2.32.90")
	require.Error(t, err)

	data, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.2.1.2.3.32.90")
	require.NoError(t, err)
	require.Equal(t, "NotAConflict", data.Name)
}

func TestResolverWithNoMatchVariableShouldStopBeforeRoot(t *testing.T) {
	logger := fxutil.Test[log.Component](t, log.MockModule)
	resolver := &MultiFilesOIDResolver{traps: make(TrapSpec), logger: logger}
	trapDB := TrapDBFileContent{
		Traps: TrapSpec{
			"1.3.6.1.6.3.1.1.5.4": TrapMetadata{Name: "linkUp", MIBName: "IF-MIB"},
		},
		Variables: variableSpec{
			"1":         VariableMetadata{Name: "should-not-resolve"},
			"1.3":       VariableMetadata{Name: "should-not-resolve"},
			"1.3.6.1":   VariableMetadata{Name: "should-not-resolve"},
			"1.3.6.1.4": VariableMetadata{Name: "should-not-resolve"},
		},
	}
	updateResolverWithIntermediateJSONReader(t, resolver, trapDB)
	_, err := resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.4.12.3.5.6")
	require.Error(t, err)

	_, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.1.10")
	require.Error(t, err)

	_, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.6.10")
	require.Error(t, err)

	_, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.3.4")
	require.Error(t, err)

	_, err = resolver.GetVariableMetadata("1.3.6.1.6.3.1.1.5.4", "1.8")
	require.Error(t, err)

}

func updateResolverWithIntermediateJSONReader(t *testing.T, oidResolver *MultiFilesOIDResolver, trapData TrapDBFileContent) {
	data, err := json.Marshal(trapData)
	require.NoError(t, err)

	reader := bytes.NewReader(data)
	err = oidResolver.updateFromReader(reader, json.Unmarshal)
	require.NoError(t, err)
}

func updateResolverWithIntermediateYAMLReader(t *testing.T, oidResolver *MultiFilesOIDResolver, trapData TrapDBFileContent) {
	data, err := yaml.Marshal(trapData)
	require.NoError(t, err)

	reader := bytes.NewReader(data)
	err = oidResolver.updateFromReader(reader, yaml.Unmarshal)
	require.NoError(t, err)
}

func TestIsValidOID_PropertyBasedTesting(t *testing.T) {
	rand.Seed(time.Now().Unix())
	testSize := 100
	validOIDs := make([]string, testSize)
	for i := 0; i < testSize; i++ {
		// Valid cases
		oidLen := rand.Intn(100) + 2
		oidParts := make([]string, oidLen)
		for j := 0; j < oidLen; j++ {
			oidParts[j] = fmt.Sprint(rand.Intn(100000))
		}
		recreatedOID := strings.Join(oidParts, ".")
		if rand.Intn(2) == 0 {
			recreatedOID = "." + recreatedOID
		}
		validOIDs[i] = recreatedOID
		require.True(t, IsValidOID(validOIDs[i]), "OID: %s", validOIDs[i])
	}

	var invalidRunes = []rune(",?><|\\}{[]()*&^%$#@!abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	for i := 0; i < testSize; i++ {
		// Valid cases
		oid := validOIDs[i]
		x := 0
		switch x = rand.Intn(3); x {
		case 0:
			// Append a dot at the end, this is not possible
			oid = oid + "."
		case 1:
			// Append a random invalid character anywhere
			randomRune := invalidRunes[rand.Intn(len(invalidRunes))]
			randomIdx := rand.Intn(len(oid))
			oid = oid[:randomIdx] + string(randomRune) + oid[randomIdx:]
		case 2:
			// Put two dots next to each other
			oidParts := strings.Split(oid, ".")
			randomIdx := rand.Intn(len(oidParts)-1) + 1
			oidParts[randomIdx] = "." + oidParts[randomIdx]
			oid = strings.Join(oidParts, ".")
		}

		require.False(t, IsValidOID(oid), "OID: %s", oid)
	}
}

func TestIsValidOID_Unit(t *testing.T) {
	cases := map[string]bool{
		"1.3.6.1.4.1.4962.2.1.6.3":       true,
		".1.3.6.1.4.1.4962.2.1.6.999999": true,
		"1":                              true,
		"1.3.6.1.4.1.4962.2.1.-6.3":      false,
		"1.3.6.1.4.1..4962.2.1.6.3":      false,
		"1.3.6.1foo.4.1.4962.2.1.6.3":    false,
		"1.3.6.1foo.4.1.4962_2.1.6.3":    false,
		"1.3.6.1.4.1.4962.2.1.6.999999.": false,
	}

	for oid, expected := range cases {
		require.Equal(t, expected, IsValidOID(oid))
	}
}
