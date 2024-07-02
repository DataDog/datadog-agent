// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main holds main related files
package main

import (
	"flag"
	"fmt"
	"go/types"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/fatih/structtag"
	"golang.org/x/tools/go/packages"
)

func FindStruct(pkg *packages.Package, name string) *types.Struct {
	for _, d := range pkg.TypesInfo.Defs {
		if d != nil && d.Name() == name {
			s, ok := d.Type().Underlying().(*types.Struct)
			if ok {
				return s
			}
		}
	}

	return nil
}

type Field struct {
	Name    string
	Handler map[string]string
}

var tmpl = `
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

package {{.Package}}

import (
	"go4.org/intern"

	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	{{- if ne .Package .StructPackage }}
	"{{.Import}}"
	{{end}}
)

var _ = intern.Value{}

func {{.Scope}} Copy(event *smodel.Event) any {
	{{- if ne .Package .StructPackage }}
	var result {{.StructPackage}}.{{.Struct}}
	{{- else -}}
	var result {{.Struct}}
	{{- end -}}

	{{- range .Fields}}
		{{ $evt := index .Handler "event" }}
		{{ $cast := index .Handler "cast" }}
		{{- if ne $evt "*" -}}
		if event.GetEventType() == smodel.{{$evt}} {
		{{- end -}}
		{{ $getter := index .Handler "getter" }}
		{{- if eq $getter "FilterEnvs"}}
		{{- $envs := index .Handler "envs" }}
		value{{.Name}} := smodel.FilterEnvs(event.GetProcessEnvp(), map[string]bool{ {{ $envs }} })
		{{- else -}}
		{{- if index .Handler "intern" -}}
		value{{.Name}} := intern.GetByString(event.{{$getter}}())
		{{- else -}}
		{{- if $cast -}}
		value{{.Name}} := {{$cast}}(event.{{$getter}}())
		{{- else -}}
		value{{.Name}} := event.{{$getter}}()
		{{- end}}
		{{- end}}
		{{- end}}
		result.{{.Name}} = value{{.Name}}
		{{- if ne $evt "*" -}}
		}
		{{- end -}}
	{{- end }}
	return &result
}
`

func getValueTag(tag *structtag.Tags, os string) string {
	cp, err := tag.Get("copy_" + os)
	if err == nil {
		if value := cp.Value(); value != "" {
			return value
		}
	}

	cp, err = tag.Get("copy")
	if err != nil {
		return ""
	}

	return cp.Value()
}

func CopyFields(s *types.Struct, os string) []Field {
	var fields []Field
	for i := 0; i < s.NumFields(); i++ {
		f := s.Field(i)

		tag, err := structtag.Parse(s.Tag(i))
		if err != nil {
			panic(err)
		}

		value := getValueTag(tag, os)
		if value == "" {
			continue
		}

		els := strings.Split(value, ";")
		handler := map[string]string{
			"getter": els[0],
		}
		if len(els) > 1 {
			for _, el := range els[1:] {
				kv := strings.Split(el, ":")
				key, value := kv[0], kv[1]

				if key == "envs" {
					envs := strings.Split(value, ",")
					value = fmt.Sprintf(`"%s": true`, strings.Join(envs, `": true, "`))
				} else if key == "cast" {
					switch value {
					case "int8", "uint8", "int16", "uint16", "int32", "uint32", "string", "[]byte":
					default:
						panic("cast not supported")
					}
				}

				handler[key] = value
			}
		}

		fields = append(fields, Field{
			Name:    f.Name(),
			Handler: handler,
		})
	}
	return fields
}

func main() {
	flagScope := flag.String("scope", "", "scope of the copy function")
	flagPkg := flag.String("pkg", "main", "output package name")
	flagOutput := flag.String("output", "", "output file name")
	flagOs := flag.String("os", "", "target os")

	flag.Parse()
	args := flag.Args()

	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "invalid number of arguments: need struct and package.\n")
		flag.Usage()
		os.Exit(1)
	}

	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedTypesInfo}
	pkgs, err := packages.Load(cfg, args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "load error: %v\n", err)
		os.Exit(1)
	}

	if len(pkgs) <= 0 {
		fmt.Fprintf(os.Stderr, "error: no package found\n")
		os.Exit(1)
	}

	pkg := pkgs[0]
	strct := FindStruct(pkg, args[0])
	if strct == nil {
		fmt.Fprintf(os.Stderr, "Error: could not find struct\n")
		os.Exit(1)
	}

	data := struct {
		Package       string
		Struct        string
		StructPackage string
		Scope         string
		Fields        []Field
		Import        string
	}{
		Package:       *flagPkg,
		Struct:        args[0],
		StructPackage: pkg.Types.Name(),
		Scope:         *flagScope,
		Fields:        CopyFields(strct, *flagOs),
		Import:        pkg.Types.Path(),
	}

	file, err := os.Create(*flagOutput)
	if err != nil {
		panic(err)
	}

	t := template.Must(template.New("copy").Parse(tmpl))
	err = t.Execute(file, data)
	if err != nil {
		panic(err)
	}

	cmd := exec.Command("gofmt", "-s", "-w", *flagOutput)
	if _, err := cmd.CombinedOutput(); err != nil {
		panic(err)
	}
}
