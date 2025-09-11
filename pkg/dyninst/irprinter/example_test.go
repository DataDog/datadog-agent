// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irprinter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// The below awk script can be used to get the appropriate output.
/*
	BEGIN {
		inside = 0
		printf ("// Output:\n")
	}

	$1 == "want:" {
		inside = 0
	}

	inside {
		printf "// %s\n", $0
	}

	$1 == "got:" {
		inside = 1
	}
*/

// This is a simple example of how to use the irprinter package.
func ExamplePrintJSON() {
	p := constructExampleProgram()

	// Marshal to JSON.
	data, err := PrintJSON(p)
	if err != nil {
		panic(err)
	}

	// Pretty print the JSON.
	var buf bytes.Buffer
	json.Indent(&buf, data, "", "  ")
	fmt.Println(buf.String())

	// Output:
	// {
	//   "ID": 1,
	//   "Probes": [],
	//   "Subprograms": [],
	//   "Types": [
	//     {
	//       "__kind": "PointerType",
	//       "ID": 3,
	//       "Name": "*Node",
	//       "ByteSize": 8,
	//       "Pointee": "1 StructureType Node"
	//     },
	//     {
	//       "__kind": "StructureType",
	//       "ID": 1,
	//       "Name": "Node",
	//       "ByteSize": 16,
	//       "RawFields": [
	//         {
	//           "Name": "value",
	//           "Offset": 0,
	//           "Type": "2 BaseType int"
	//         },
	//         {
	//           "Name": "next",
	//           "Offset": 8,
	//           "Type": "3 PointerType *Node"
	//         }
	//       ]
	//     },
	//     {
	//       "__kind": "BaseType",
	//       "ID": 2,
	//       "Name": "int",
	//       "ByteSize": 8,
	//       "GoKind": 2
	//     }
	//   ],
	//   "MaxTypeID": 3,
	//   "Issues": [],
	//   "GoModuledataInfo": {
	//     "FirstModuledataAddr": "0xdeadbeef",
	//     "TypesOffset": 1234
	//   },
	//   "CommonTypes": {
	//     "G": "4 StructureType runtime.g",
	//     "M": "5 StructureType runtime.m"
	//   }
	// }
}

func ExamplePrintYAML() {
	p := constructExampleProgram()

	yamlData, err := PrintYAML(p)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(yamlData))

	// Output:
	// ID: 1
	// Probes: []
	// Subprograms: []
	// Types:
	//     - __kind: PointerType
	//       ID: 3
	//       Name: '*Node'
	//       ByteSize: 8
	//       Pointee: 1 StructureType Node
	//     - __kind: StructureType
	//       ID: 1
	//       Name: Node
	//       ByteSize: 16
	//       RawFields:
	//         - Name: value
	//           Offset: 0
	//           Type: 2 BaseType int
	//         - Name: next
	//           Offset: 8
	//           Type: 3 PointerType *Node
	//     - __kind: BaseType
	//       ID: 2
	//       Name: int
	//       ByteSize: 8
	//       GoKind: 2
	// MaxTypeID: 3
	// Issues: []
	// GoModuledataInfo: {FirstModuledataAddr: "0xdeadbeef", TypesOffset: 1234}
	// CommonTypes:
	//     G: 4 StructureType runtime.g
	//     M: 5 StructureType runtime.m
}

func constructExampleProgram() *ir.Program {
	// Create a simple program with a cyclic type
	p := &ir.Program{
		ID:        1,
		MaxTypeID: 3,
		Types:     make(map[ir.TypeID]ir.Type),
		GoModuledataInfo: ir.GoModuledataInfo{
			FirstModuledataAddr: 0xdeadbeef,
			TypesOffset:         1234,
		},
		CommonTypes: ir.CommonTypes{
			G: &ir.StructureType{
				TypeCommon: ir.TypeCommon{
					ID:       4,
					Name:     "runtime.g",
					ByteSize: 128,
				},
			},
			M: &ir.StructureType{
				TypeCommon: ir.TypeCommon{
					ID:       5,
					Name:     "runtime.m",
					ByteSize: 128,
				},
			},
		},
	}

	// Create a self-referential linked list node
	nodeStruct := &ir.StructureType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "Node",
			ByteSize: 16,
		},
		RawFields: []ir.Field{
			{
				Name:   "value",
				Offset: 0,
				Type: &ir.BaseType{
					TypeCommon: ir.TypeCommon{
						ID:       2,
						Name:     "int",
						ByteSize: 8,
					},
					GoTypeAttributes: ir.GoTypeAttributes{
						GoKind: reflect.Int,
					},
				},
			},
		},
	}
	p.Types[1] = nodeStruct
	p.Types[2] = nodeStruct.RawFields[0].Type

	// Create pointer to Node
	nodePtr := &ir.PointerType{
		TypeCommon: ir.TypeCommon{
			ID:       3,
			Name:     "*Node",
			ByteSize: 8,
		},
		Pointee: nodeStruct,
	}
	p.Types[3] = nodePtr

	// Complete the cycle
	nodeStruct.RawFields = append(nodeStruct.RawFields, ir.Field{
		Name:   "next",
		Offset: 8,
		Type:   nodePtr,
	})
	return p
}
