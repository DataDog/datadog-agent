// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build ignore

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func main() {
	if len(os.Args[1:]) != 2 {
		panic("please use 'go run render_config.go <template_file> <destination_file>'")
	}

	tplFile, _ := filepath.Abs(os.Args[1])
	tplFilename := filepath.Base(tplFile)
	destFile, _ := filepath.Abs(os.Args[2])

	f, err := os.Create(destFile)
	if err != nil {
		panic(err)
	}

	funcMap := template.FuncMap{
		"prefix": prefix,
		"dict":   dict,
	}

	t := template.Must(template.New(tplFilename).Funcs(funcMap).ParseFiles(tplFile))
	err = t.Execute(f, config.GetGroups())
	if err != nil {
		panic(err)
	}

	fmt.Println("Successfully wrote", destFile)
}

func prefix(pad, v string) string {
	return pad + strings.Replace(strings.TrimPrefix(v, "\n"), "\n", "\n"+pad, -1)
}

func dict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, errors.New("invalid dict call")
	}
	dict := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, errors.New("dict keys must be strings")
		}
		dict[key] = values[i+1]
	}
	return dict, nil
}
