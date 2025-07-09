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

	// Marshal to JSON
	data, err := PrintJSON(p)
	if err != nil {
		panic(err)
	}

	// Pretty print the JSON
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
	//       "__kind": "StructureType",
	//       "ID": 1,
	//       "Name": "Node",
	//       "ByteSize": 16,
	//       "Fields": [
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
	//     },
	//     {
	//       "__kind": "PointerType",
	//       "ID": 3,
	//       "Name": "*Node",
	//       "ByteSize": 8,
	//       "Pointee": "1 StructureType Node"
	//     }
	//   ],
	//   "MaxTypeID": 3,
	//   "Issues": []
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
	//     - __kind: StructureType
	//       ID: 1
	//       Name: Node
	//       ByteSize: 16
	//       Fields:
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
	//     - __kind: PointerType
	//       ID: 3
	//       Name: '*Node'
	//       ByteSize: 8
	//       Pointee: 1 StructureType Node
	// MaxTypeID: 3
	// Issues: []
}

func constructExampleProgram() *ir.Program {
	// Create a simple program with a cyclic type
	p := &ir.Program{
		ID:        1,
		MaxTypeID: 3,
		Types:     make(map[ir.TypeID]ir.Type),
	}

	// Create a self-referential linked list node
	nodeStruct := &ir.StructureType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "Node",
			ByteSize: 16,
		},
		Fields: []ir.Field{
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
	p.Types[2] = nodeStruct.Fields[0].Type

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
	nodeStruct.Fields = append(nodeStruct.Fields, ir.Field{
		Name:   "next",
		Offset: 8,
		Type:   nodePtr,
	})
	return p
}
