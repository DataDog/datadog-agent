package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/types"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
	"unicode"

	"github.com/davecgh/go-spew/spew"
	"golang.org/x/tools/go/loader"
)

const (
	pkgPrefix = "github.com/DataDog/datadog-agent/pkg/security"
)

var (
	filename string
	pkgname  string
	output   string
	strict   bool
	verbose  bool
	program  *loader.Program
	packages map[string]*types.Package
)

type Module struct {
	Name      string
	PkgPrefix string
	BuildTags []string
	Fields    map[string]*field
}

var module *Module

type field struct {
	Name    string
	Type    string
	IsArray bool
	Public  bool
	Tags    string
}

func (f *field) ElemType() string {
	return strings.TrimLeft(f.Type, "[]")
}

type accessor struct {
	Name    string
	IsArray bool
	Fields  []field
}

func (g *accessor) Has(kind string) bool {
	for _, field := range g.Fields {
		if field.Type == kind {
			return true
		}
	}
	return false
}

func resolveSymbol(pkg, symbol string) (types.Object, error) {
	if typePackage, found := packages[pkg]; found {
		return typePackage.Scope().Lookup(symbol), nil
	}

	return nil, fmt.Errorf("Failed to retrieve package info for %s", pkg)
}

func handleBasic(name, alias, kind string, tags string) {
	fmt.Printf("handleBasic %s %s\n", name, kind)

	switch kind {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		module.Fields[alias] = &field{Name: name, Type: "int", Public: true}
	default:
		public := false
		firstChar := strings.TrimPrefix(kind, "[]")
		if splits := strings.Split(firstChar, "."); len(splits) > 1 {
			firstChar = splits[len(splits)-1]
		}
		if unicode.IsUpper(rune(firstChar[0])) {
			public = true
		}
		module.Fields[alias] = &field{
			Name:    name,
			Type:    kind,
			IsArray: strings.HasPrefix(kind, "[]"),
			Public:  public,
			Tags:    tags,
		}
	}
}

func handleField(astFile *ast.File, name, alias, prefix, aliasPrefix, pkgName string, fieldType *ast.Ident, tags string) error { // fmt.Printf("handleField fieldName %s, ident %s, pkgName %s, prefix %s, fieldType, %s\n", fieldName, ident, pkgName, prefix, fieldType)
	fmt.Printf("handleField fieldName %s, alias %s, prefix %s, aliasPrefix %s, pkgName %s, fieldType, %s\n", name, alias, prefix, aliasPrefix, pkgName, fieldType)

	switch fieldType.Name {
	case "string", "bool", "int", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64":
		if prefix != "" {
			name = prefix + "." + name
			alias = aliasPrefix + "." + alias
		}
		handleBasic(name, alias, fieldType.Name, tags)
	default:
		symbol, err := resolveSymbol(pkgName, fieldType.Name)
		if err != nil {
			return fmt.Errorf("Failed to resolve symbol for %+v: %s", fieldType, err)
		}
		if symbol == nil {
			return fmt.Errorf("Failed to resolve symbol for %+v", fieldType)
		}

		if prefix != "" {
			prefix = prefix + "." + name
			aliasPrefix = aliasPrefix + "." + alias
		} else {
			prefix = name
			aliasPrefix = alias
		}

		spec := astFile.Scope.Lookup(fieldType.Name)
		handleSpec(astFile, spec.Decl, prefix, aliasPrefix)
	}

	return nil
}

func handleSpec(astFile *ast.File, spec interface{}, prefix, aliasPrefix string) {
	fmt.Printf("handleSpec spec: %+v, prefix: %s, aliasPrefix %s\n", spec, prefix, aliasPrefix)

	if typeSpec, ok := spec.(*ast.TypeSpec); ok {
		if structType, ok := typeSpec.Type.(*ast.StructType); ok {
		FIELD:
			for _, field := range structType.Fields.List {
				if len(field.Names) > 0 {
					fieldName := field.Names[0].Name
					fieldAlias := fieldName
					var tag reflect.StructTag
					if field.Tag != nil {
						tag = reflect.StructTag(field.Tag.Value[1 : len(field.Tag.Value)-1])
					}

					tags, found := tag.Lookup("tags")
					if found {
						f := func(c rune) bool {
							return !unicode.IsLetter(c) && !unicode.IsNumber(c)
						}
						tags = fmt.Sprintf(`"%s"`, strings.Join(strings.FieldsFunc(tags, f), `","`))
					}

					if fieldTag, found := tag.Lookup("field"); found {
						if fieldTag == "omit" {
							continue FIELD
						}
						fieldAlias = fieldTag
					}

					if fieldType, ok := field.Type.(*ast.Ident); ok {
						if err := handleField(astFile, fieldName, fieldAlias, prefix, aliasPrefix, filepath.Base(pkgname), fieldType, tags); err != nil {
							log.Print(err)
						}
						continue
					} else if fieldType, ok := field.Type.(*ast.StarExpr); ok {
						if itemIdent, ok := fieldType.X.(*ast.Ident); ok {
							handleField(astFile, fieldName, fieldAlias, prefix, aliasPrefix, filepath.Base(pkgname), itemIdent, tags)
							continue
						}
					}

					if strict {
						log.Panicf("Don't know what to do with %s: %s", fieldName, spew.Sdump(field.Type))
					}
					if verbose {
						log.Printf("Don't know what to do with %s: %s", fieldName, spew.Sdump(field.Type))
					}
				} else {
					// Embedded field
					ident, _ := field.Type.(*ast.Ident)
					if starExpr, ok := field.Type.(*ast.StarExpr); ident == nil && ok {
						ident, _ = starExpr.X.(*ast.Ident)
					}

					if ident != nil {
						embedded := astFile.Scope.Lookup(ident.Name)
						if embedded != nil {
							handleSpec(astFile, embedded.Decl, prefix, aliasPrefix)
						}
					}
				}
			}
		} else {
			log.Printf("Don't know what to do with %s (%s)", typeSpec.Name, spew.Sdump(typeSpec))
		}
	}
}

func parseFile(filename string, pkgName string) (*Module, error) {
	conf := loader.Config{
		ParserMode:  parser.ParseComments,
		AllowErrors: true,
		TypeChecker: types.Config{
			Error: func(err error) {
				if verbose {
					log.Print(err)
				}
			},
		},
	}

	astFile, err := conf.ParseFile(filename, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse %s: %s", filename, err)
	}

	conf.Import(pkgName)

	program, err = conf.Load()
	if err != nil {
		return nil, fmt.Errorf("Failed to load %s(%s): %s", filename, pkgName, err)
	}

	packages = make(map[string]*types.Package, len(program.AllPackages))
	for typePackage := range program.AllPackages {
		packages[typePackage.Name()] = typePackage
	}

	var buildTags []string
	for _, comment := range astFile.Comments {
		if strings.HasPrefix(comment.Text(), "+build ") {
			buildTags = append(buildTags, comment.Text())
		}
	}

	module = &Module{
		Name:      astFile.Name.Name,
		PkgPrefix: pkgPrefix,
		BuildTags: buildTags,
		Fields:    make(map[string]*field),
	}

	for _, decl := range astFile.Decls {
		if decl, ok := decl.(*ast.GenDecl); ok {
			genaccessors := false
			if decl.Doc != nil {
				for _, doc := range decl.Doc.List {
					if genaccessors = strings.Index(doc.Text, "genaccessors") != -1; genaccessors {
						break
					}
				}
			}
			if !genaccessors {
				continue
			}

			for _, spec := range decl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					handleSpec(astFile, typeSpec, "", "")
				}
			}
		}
	}

	return module, nil
}

func main() {
	inputInfo, err := os.Stat(filename)
	if err != nil {
		panic(err)
	}

	if output == "" {
		output = strings.TrimSuffix(filename, ".go") + "_genaccessors.go"
	}

	outputInfo, err := os.Stat(output)
	if err == nil {
		if inputInfo.ModTime().Before(outputInfo.ModTime()) {
			// Skipping file
			if verbose {
				log.Printf("Skipping %s as %s is newer", filename, output)
			}
			return
		}
	}

	tmpl := template.Must(template.New("header").Parse(`{{- range .BuildTags }}// {{.}}{{end}}

// Code generated - DO NOT EDIT.

package	eval

import (
	"github.com/pkg/errors"
)

var (
	ErrFieldNotFound = errors.New("field not found")
)

func GetAccessor(key string) (interface{}, []string, error) {
	switch key {
	{{range $Name, $Field := .Fields}}
	case "{{$Name}}":
	{{if eq $Field.Type "string"}}
		return &StringEvaluator{
			Eval: func(ctx *Context) string { return ctx.Event.{{$Field.Name}} },
			DebugEval: func(ctx *Context) string { return ctx.Event.{{$Field.Name}} },
			Field: key,
		}, []string{ {{$Field.Tags}} }, nil
	{{else if eq $Field.Type "stringer"}}
		return &StringEvaluator{
			Eval: func(ctx *Context) string { return ctx.Event.{{$Field.Name}}.String() },
			DebugEval: func(ctx *Context) string { return ctx.Event.{{$Field.Name}}.String() },
			Field: key,
		}, []string{ {{$Field.Tags}} }, nil
	{{else if eq .Type "int"}}
	return &IntEvaluator{
		Eval: func(ctx *Context) int { return int(ctx.Event.{{$Field.Name}}) },
		DebugEval: func(ctx *Context) int { return int(ctx.Event.{{$Field.Name}}) },
		Field: key,
	}, []string{ {{$Field.Tags}} }, nil
	{{else if eq .Type "bool"}}
	return &BoolEvaluator{
		Eval: func(ctx *Context) bool { return ctx.Event.{{$Field.Name}} },
		DebugEval: func(ctx *Context) bool { return ctx.Event.{{$Field.Name}} },
		Field: key,
	}, []string{ {{$Field.Tags}} }, nil
	{{end}}{{end}}
	}

	return nil, nil, errors.Wrap(ErrFieldNotFound, key)
}
`))

	module, err = parseFile(filename, pkgname)
	if err != nil {
		panic(err)
	}

	outputFile, err := os.Create(output)
	if err != nil {
		panic(err)
	}

	if err := tmpl.Execute(outputFile, module); err != nil {
		panic(err)
	}

	if err := outputFile.Close(); err != nil {
		panic(err)
	}

	cmd := exec.Command("gofmt", "-s", "-w", output)
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

func init() {
	flag.BoolVar(&verbose, "verbose", false, "Be verbose")
	flag.StringVar(&filename, "filename", os.Getenv("GOFILE"), "Go file to generate decoders from")
	flag.StringVar(&pkgname, "package", pkgPrefix+"/"+os.Getenv("GOPACKAGE"), "Go package name")
	flag.StringVar(&output, "output", "", "Go generated file")
	flag.Parse()
}
