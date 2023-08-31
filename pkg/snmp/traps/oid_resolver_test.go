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

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
		"1.3.6.1.2.1.2.2.1.7":      VariableMetadata{Name: "ifAdminStatus", Enumeration: map[int]string{1: "up", 2: "down", 3: "testing"}},
		"1.3.6.1.2.1.2.2.1.8":      VariableMetadata{Name: "ifOperStatus", Enumeration: map[int]string{1: "up", 2: "down", 3: "testing", 4: "unknown", 5: "dormant", 6: "notPresent", 7: "lowerLayerDown"}},
		"1.3.6.1.4.1.8072.2.3.2.1": VariableMetadata{Name: "netSnmpExampleHeartbeatRate"},
		"1.3.6.1.2.1.200.1.1.1.3": VariableMetadata{Name: "pwCepSonetConfigErrorOrStatus", Bits: map[int]string{
			0:  "other",
			1:  "timeslotInUse",
			2:  "timeslotMisuse",
			3:  "peerDbaIncompatible",
			4:  "peerEbmIncompatible",
			5:  "peerRtpIncompatible",
			6:  "peerAsyncIncompatible",
			7:  "peerDbaAsymmetric",
			8:  "peerEbmAsymmetric",
			9:  "peerRtpAsymmetric",
			10: "peerAsyncAsymmetric",
		}},
		"1.3.6.1.2.1.200.1.3.1.5": VariableMetadata{Name: "myFakeVarType", Bits: map[int]string{
			0:   "test0",
			1:   "test1",
			3:   "test3",
			5:   "test5",
			6:   "test6",
			12:  "test12",
			15:  "test15",
			130: "test130",
		}},
		"1.3.6.1.2.1.200.1.3.1.6": VariableMetadata{
			Name: "myBadVarType",
			Enumeration: map[int]string{
				0:   "test0",
				1:   "test1",
				3:   "test3",
				5:   "test5",
				6:   "test6",
				12:  "test12",
				15:  "test15",
				130: "test130",
			},
			Bits: map[int]string{
				0:   "test0",
				1:   "test1",
				3:   "test3",
				5:   "test5",
				6:   "test6",
				12:  "test12",
				15:  "test15",
				130: "test130",
			},
		},
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
	logger := fxutil.Test[log.Component](t, log.MockModule)
	resolver := &MultiFilesOIDResolver{traps: make(TrapSpec), logger: logger}
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
	logger := fxutil.Test[log.Component](t, log.MockModule)
	resolver := &MultiFilesOIDResolver{traps: make(TrapSpec), logger: logger}
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
	trapDB := trapDBFileContent{
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
	trapDB := trapDBFileContent{
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
