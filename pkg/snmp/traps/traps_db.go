// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package traps

// VariableMetadata is the MIB-extracted information of a given trap variable
type VariableMetadata struct {
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"descr" json:"descr"`
	Mapping     map[int]string `yaml:"map" json:"map"`
}

// variableSpec contains the variableMetadata for each known variable of a given trap db file
type variableSpec map[string]VariableMetadata

// TrapMetadata is the MIB-extracted information of a given trap OID.
// It also contains a reference to the variableSpec that was defined in the same trap db file.
// This is to prevent variable conflicts and to give precedence to the variable definitions located]
// in the same trap db file as the trap.
type TrapMetadata struct {
	Name            string `yaml:"name" json:"name"`
	MIBName         string `yaml:"mib" json:"mib"`
	Description     string `yaml:"descr" json:"descr"`
	variableSpecPtr variableSpec
}

// TrapSpec contains the variableMetadata for each known trap in all trap db files
type TrapSpec map[string]TrapMetadata

type trapDBFileContent struct {
	Traps     TrapSpec     `yaml:"traps" json:"traps"`
	Variables variableSpec `yaml:"vars" json:"vars"`
}
