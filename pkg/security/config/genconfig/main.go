package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/security/config"
)

func main() {
	var output string

	flag.StringVar(&output, "output", "", "Path of the file to be written. Defaults to standard output")
	flag.Parse()

	var outputFile *os.File
	var err error

	if output != "" {
		if outputFile, err = os.Create(output); err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
		defer outputFile.Close()
	} else {
		outputFile = os.Stdout
	}

	if _, err := outputFile.WriteString(config.DefaultPolicy); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}
