// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package traps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

var dummyTrapDB = trapDBFileContent{
	Traps: TrapSpec{
		"1.3.6.1.6.3.1.1.5.3":      TrapMetadata{Name: "ifDown", MIBName: "IF-MIB"},                                             // v1 Trap
		"1.3.6.1.4.1.8072.2.3.0.1": TrapMetadata{Name: "netSnmpExampleHeartbeatNotification", MIBName: "NET-SNMP-EXAMPLES-MIB"}, // v2+
		"1.3.6.1.6.3.1.1.5.4":      TrapMetadata{Name: "linkUp", MIBName: "IF-MIB"},
	},
	Variables: variableSpec{
		"1.3.6.1.2.1.2.2.1.1":      VariableMetadata{Name: "ifIndex"},
		"1.3.6.1.2.1.2.2.1.7":      VariableMetadata{Name: "ifAdminStatus", Mapping: map[int]string{1: "up", 2: "down", 3: "testing"}},
		"1.3.6.1.2.1.2.2.1.8":      VariableMetadata{Name: "ifOperStatus", Mapping: map[int]string{1: "up", 2: "down", 3: "testing", 4: "unknown", 5: "dormant", 6: "notPresent", 7: "lowerLayerDown"}},
		"1.3.6.1.4.1.8072.2.3.2.1": VariableMetadata{Name: "netSnmpExampleHeartbeatRate"},
	},
}

var resolverWithData = &MockedResolver{content: dummyTrapDB}

type MockedResolver struct {
	content trapDBFileContent
}

func (r MockedResolver) GetTrapMetadata(trapOid string) (TrapMetadata, error) {
	trapOid = NormalizeOID(trapOid)
	trapData, ok := r.content.Traps[trapOid]
	if !ok {
		return TrapMetadata{}, fmt.Errorf("trap OID %s is not defined", trapOid)
	}
	return trapData, nil
}
func (r MockedResolver) GetVariableMetadata(string, varOid string) (VariableMetadata, error) {
	varOid = NormalizeOID(varOid)
	varData, ok := r.content.Variables[varOid]
	if !ok {
		return VariableMetadata{}, fmt.Errorf("variable OID %s is not defined", varOid)
	}
	return varData, nil
}

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
	trapDBFile := &trapDBFileContent{
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
				Mapping:     map[int]string{2: "test"},
			},
		},
	}
	data, err := json.Marshal(trapDBFile)
	require.NoError(t, err)
	require.Equal(t, []byte("{\"traps\":{\"foo\":{\"name\":\"xx\",\"mib\":\"yy\",\"descr\":\"\"}},\"vars\":{\"bar\":{\"name\":\"yy\",\"descr\":\"dummy description\",\"map\":{\"2\":\"test\"}}}}"), data)
	err = json.Unmarshal([]byte("{\"traps\": {\"1.2\": {\"name\": \"dd\"}}}"), &trapDBFile)
	require.NoError(t, err)
}

func TestSortFiles(t *testing.T) {
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
	sortedFiles := getSortedFileNames(files)
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
	resolver := &MultiFilesOIDResolver{traps: make(TrapSpec)}
	trapData := trapDBFileContent{
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
	resolver := &MultiFilesOIDResolver{traps: make(TrapSpec)}
	trapDataA := trapDBFileContent{
		Traps: TrapSpec{"1.3.6.1.4.1.8072.2.3.0.1": TrapMetadata{Name: "foo", MIBName: "FOO-MIB"}},
	}
	trapDataB := trapDBFileContent{
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
	resolver := &MultiFilesOIDResolver{traps: make(TrapSpec)}
	trapDataA := trapDBFileContent{
		Traps: TrapSpec{"1.3.6.1.4.1.8072.2.3.0.1": TrapMetadata{}},
		Variables: variableSpec{
			"1.3.6.1.4.1.8072.2.3.2.1": VariableMetadata{
				Name: "netSnmpExampleHeartbeatRate",
			},
		},
	}
	trapDataB := trapDBFileContent{
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

func updateResolverWithIntermediateJSONReader(t *testing.T, oidResolver *MultiFilesOIDResolver, trapData trapDBFileContent) {
	data, err := json.Marshal(trapData)
	require.NoError(t, err)

	reader := bytes.NewReader(data)
	err = oidResolver.updateFromReader(reader, json.Unmarshal)
	require.NoError(t, err)
}

func updateResolverWithIntermediateYAMLReader(t *testing.T, oidResolver *MultiFilesOIDResolver, trapData trapDBFileContent) {
	data, err := yaml.Marshal(trapData)
	require.NoError(t, err)

	reader := bytes.NewReader(data)
	err = oidResolver.updateFromReader(reader, yaml.Unmarshal)
	require.NoError(t, err)
}
