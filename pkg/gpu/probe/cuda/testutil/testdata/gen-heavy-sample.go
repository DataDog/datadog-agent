// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package main is the entry point for the heavy sample template generator
package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"text/template"
)

var templatePath = flag.String("template", "heavy-sample.cu.tmpl", "Path to the template file")
var outputPath = flag.String("output", "heavy-sample.cu", "Path to the output file")
var numKernels = flag.Int("kernels", 80, "Number of kernels to generate")
var numVariablesPerKernel = flag.Int("variables", 10, "Number of variables to generate per kernel")

// Note that the number of instructions per kernel doesn't really affect our fatbin parser, but it does increase the binary size, so it's
// useful to keep it low to avoid committing large binaries to the repository
var numInstructionsPerKernel = flag.Int("instructions", 10, "Number of instructions to generate per kernel")
var sharedMemorySize = flag.Int("shared-memory", 1024, "Size of the shared memory in bytes")

const sharedMemoryVar = "myVar"

// Kernel represents a kernel entry in the template
type Kernel struct {
	Name             string
	Argdef           string
	Argcall          string
	Instructions     []string
	SharedMemorySize int
}

// Variable represents a variable entry in the template that will be shared between kernels
type Variable struct {
	Name string
	Type string
}

// TemplateData represents the data that will be passed to the template
type TemplateData struct {
	Kernels   []Kernel
	Variables []Variable
}

func genInstructions(numInstructions int) []string {
	instructions := make([]string, numInstructions)
	for i := 0; i < numInstructions; i++ {
		indexSrc := rand.IntN(10)
		valueMult := rand.Float64() * 50
		instructions[i] = fmt.Sprintf("%s[%d] = %f * %s[threadIdx.x];", sharedMemoryVar, indexSrc, valueMult, sharedMemoryVar)
	}
	return instructions
}

func main() {
	flag.Parse()

	// Load the template file
	tmpl, err := template.New(*templatePath).ParseFiles(*templatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing template file: %v\n", err)
		os.Exit(1)
	}

	// Create the output file
	out, err := os.Create(*outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()

	var data TemplateData
	for i := 0; i < *numKernels; i++ {
		kernel := Kernel{
			Name:             fmt.Sprintf("kernel_%d", i),
			Instructions:     genInstructions(*numInstructionsPerKernel),
			SharedMemorySize: *sharedMemorySize,
		}

		var argDef []string
		var argCall []string

		for j := 0; j < *numVariablesPerKernel; j++ {
			varName := fmt.Sprintf("var_%d_%d", i, j)
			varType := "float *"
			data.Variables = append(data.Variables, Variable{Name: varName, Type: varType})
			argDef = append(argDef, fmt.Sprintf("%s %s", varType, varName))
			argCall = append(argCall, fmt.Sprintf("d_%s", varName))

			// Add data to this variable from the shared memory, to force the compiler to keep it
			kernel.Instructions = append(kernel.Instructions, fmt.Sprintf("%s[%d] = %s[%d];", varName, j, sharedMemoryVar, j))
		}

		kernel.Argdef = strings.Join(argDef, ", ")
		kernel.Argcall = strings.Join(argCall, ", ")

		data.Kernels = append(data.Kernels, kernel)
	}

	// Execute the template
	err = tmpl.Execute(out, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing template: %v\n", err)
		os.Exit(1)
	}
}
