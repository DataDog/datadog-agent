// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package oidresolverimpl

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
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver"
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

func blankResolver(t testing.TB) *multiFilesOIDResolver {
	logger := fxutil.Test[log.Component](t, logimpl.MockModule())
	return &multiFilesOIDResolver{traps: make(oidresolver.TrapSpec), logger: logger}
}

func TestDecoding(t *testing.T) {
	trapDBFile := &oidresolver.TrapDBFileContent{
		Traps: oidresolver.TrapSpec{
			"foo": oidresolver.TrapMetadata{
				Name:    "xx",
				MIBName: "yy",
			},
		},
		Variables: oidresolver.VariableSpec{
			"bar": oidresolver.VariableMetadata{
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
	logger := fxutil.Test[log.Component](t, logimpl.MockModule())
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
	resolver := blankResolver(t)
	trapData := oidresolver.TrapDBFileContent{
		Traps: oidresolver.TrapSpec{"1.3.6.1.4.1.8072.2.3.0.1": oidresolver.TrapMetadata{Name: "netSnmpExampleHeartbeat", MIBName: "NET-SNMP-EXAMPLES-MIB"}},
		Variables: oidresolver.VariableSpec{
			"1.3.6.1.4.1.8072.2.3.2.1": oidresolver.VariableMetadata{
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
	resolver := blankResolver(t)
	trapDataA := oidresolver.TrapDBFileContent{
		Traps: oidresolver.TrapSpec{"1.3.6.1.4.1.8072.2.3.0.1": oidresolver.TrapMetadata{Name: "foo", MIBName: "FOO-MIB"}},
	}
	trapDataB := oidresolver.TrapDBFileContent{
		Traps: oidresolver.TrapSpec{"1.3.6.1.4.1.8072.2.3.0.1": oidresolver.TrapMetadata{Name: "bar", MIBName: "BAR-MIB"}},
	}
	updateResolverWithIntermediateJSONReader(t, resolver, trapDataA)
	updateResolverWithIntermediateYAMLReader(t, resolver, trapDataB)
	data, err := resolver.GetTrapMetadata("1.3.6.1.4.1.8072.2.3.0.1")
	require.NoError(t, err)
	require.Equal(t, "bar", data.Name)
	require.Equal(t, "BAR-MIB", data.MIBName)
}

func TestResolverWithConflictingVariables(t *testing.T) {
	resolver := blankResolver(t)
	trapDataA := oidresolver.TrapDBFileContent{
		Traps: oidresolver.TrapSpec{"1.3.6.1.4.1.8072.2.3.0.1": oidresolver.TrapMetadata{}},
		Variables: oidresolver.VariableSpec{
			"1.3.6.1.4.1.8072.2.3.2.1": oidresolver.VariableMetadata{
				Name: "netSnmpExampleHeartbeatRate",
			},
		},
	}
	trapDataB := oidresolver.TrapDBFileContent{
		Traps: oidresolver.TrapSpec{"1.3.6.1.4.1.8072.2.3.0.2": oidresolver.TrapMetadata{}},
		Variables: oidresolver.VariableSpec{
			"1.3.6.1.4.1.8072.2.3.2.1": oidresolver.VariableMetadata{
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
	resolver := blankResolver(t)
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
	resolver := blankResolver(t)
	trapDB := oidresolver.TrapDBFileContent{
		Traps: oidresolver.TrapSpec{
			"1.3.6.1.6.3.1.1.5.4": oidresolver.TrapMetadata{Name: "linkUp", MIBName: "IF-MIB"},
		},
		Variables: oidresolver.VariableSpec{
			"1.3.6.1.2.1.2.2":     oidresolver.VariableMetadata{Name: "NodeConflict"},
			"1.3.6.1.2.1.2.2.1.1": oidresolver.VariableMetadata{Name: "ifIndex"},
			"1.3.6.1.2.1.2.3":     oidresolver.VariableMetadata{Name: "NotAConflict"},
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
	resolver := blankResolver(t)
	trapDB := oidresolver.TrapDBFileContent{
		Traps: oidresolver.TrapSpec{
			"1.3.6.1.6.3.1.1.5.4": oidresolver.TrapMetadata{Name: "linkUp", MIBName: "IF-MIB"},
		},
		Variables: oidresolver.VariableSpec{
			"1":         oidresolver.VariableMetadata{Name: "should-not-resolve"},
			"1.3":       oidresolver.VariableMetadata{Name: "should-not-resolve"},
			"1.3.6.1":   oidresolver.VariableMetadata{Name: "should-not-resolve"},
			"1.3.6.1.4": oidresolver.VariableMetadata{Name: "should-not-resolve"},
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

func updateResolverWithIntermediateJSONReader(t *testing.T, oidResolver *multiFilesOIDResolver, trapData oidresolver.TrapDBFileContent) {
	data, err := json.Marshal(trapData)
	require.NoError(t, err)

	reader := bytes.NewReader(data)
	err = oidResolver.updateFromReader(reader, json.Unmarshal)
	require.NoError(t, err)
}

func updateResolverWithIntermediateYAMLReader(t *testing.T, oidResolver *multiFilesOIDResolver, trapData oidresolver.TrapDBFileContent) {
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
		require.True(t, oidresolver.IsValidOID(validOIDs[i]), "OID: %s", validOIDs[i])
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

		require.False(t, oidresolver.IsValidOID(oid), "OID: %s", oid)
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
		require.Equal(t, expected, oidresolver.IsValidOID(oid))
	}
}
