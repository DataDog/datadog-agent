// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const editProtector = "// Code generated - DO NOT EDIT.\n"

func main() {
	var (
		input   string
		output  string
		pkgName string
	)

	flag.StringVar(&input, "input", "", "Go tests folder")
	flag.StringVar(&output, "output", "", "Go embeded tests output folder")
	flag.StringVar(&pkgName, "pkg_name", "embed_tests", "Output package name")
	flag.Parse()

	if input == "" || output == "" {
		panic(errors.New("please provide an input and output directory"))
	}

	os.RemoveAll(output)
	if err := prepareOutputDir(output); err != nil {
		panic(err)
	}

	fset := token.NewFileSet()

	if err := filepath.Walk(input, func(filepath string, info fs.FileInfo, err error) error {
		opts := NewEmbedFileOptions(filepath, input, output)
		if shouldKeepVerbatim(filepath) {
			if err := embedVerbatimFile(opts, pkgName); err != nil {
				return err
			}
		} else if strings.HasSuffix(path.Base(filepath), "_test.go") {
			if err := embedTestFile(fset, opts, pkgName); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		panic(err)
	}
}

func prepareOutputDir(outputDir string) error {
	if err := os.Mkdir(outputDir, 0755); err != nil {
		return err
	}

	gitignoreContent := "testsuite\n"
	return ioutil.WriteFile(path.Join(outputDir, ".gitignore"), []byte(gitignoreContent), 0644)
}

type EmbedFileOptions struct {
	inputPath  string
	outputPath string
}

func NewEmbedFileOptions(inputPath, inputDir, outputDir string) EmbedFileOptions {
	fileName := strings.TrimPrefix(inputPath, inputDir)
	outputPath := path.Join(outputDir, fileName)
	return EmbedFileOptions{
		inputPath:  inputPath,
		outputPath: outputPath,
	}
}

var packageNameReplacer = regexp.MustCompile(`(?m)^package tests\s*$`)

func embedVerbatimFile(opts EmbedFileOptions, pkgName string) error {
	content, err := ioutil.ReadFile(opts.inputPath)
	if err != nil {
		return err
	}

	content = packageNameReplacer.ReplaceAll(content, []byte(fmt.Sprintf("package %v\n", pkgName)))

	// create all needed subdirectories
	dirPath := path.Dir(opts.outputPath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}

	prefixedContent := append([]byte(editProtector), content...)
	return ioutil.WriteFile(opts.outputPath, prefixedContent, 0666)
}

func embedTestFile(fset *token.FileSet, opts EmbedFileOptions, pkgName string) error {
	node, err := parser.ParseFile(fset, opts.inputPath, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	shouldExportEmbedFile := false
	resDecls := make([]ast.Decl, 0, len(node.Decls))
	for _, decl := range node.Decls {
		keep := false
		funcDecl, ok := decl.(*ast.FuncDecl)
		if ok {
			info := funcDeclKeepInfo(fset, funcDecl)
			keep = info.keep
			if info.isTest {
				shouldExportEmbedFile = true
			}
		} else {
			keep = true
		}

		if keep {
			resDecls = append(resDecls, decl)
		}
	}
	node.Decls = resDecls

	if shouldExportEmbedFile {
		node.Name = ast.NewIdent(pkgName)

		// create all needed subdirectories
		dirPath := path.Dir(opts.outputPath)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return err
		}

		f, err := os.Create(opts.outputPath)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := f.WriteString(editProtector); err != nil {
			return err
		}

		if err := printer.Fprint(f, fset, node); err != nil {
			return err
		}
	}

	return nil
}

var embedCmdRegex = regexp.MustCompile(`//\s*test:embed`)

type FuncDeclKeepInfo struct {
	keep   bool
	isTest bool
}

func funcDeclKeepInfo(fset *token.FileSet, funcDecl *ast.FuncDecl) FuncDeclKeepInfo {
	if isTestFunction(fset, funcDecl) {
		if funcDecl.Doc != nil {
			for _, comment := range funcDecl.Doc.List {
				if embedCmdRegex.MatchString(comment.Text) {
					return FuncDeclKeepInfo{
						keep:   true,
						isTest: true,
					}
				}
			}
		}
		return FuncDeclKeepInfo{
			keep:   false,
			isTest: false,
		}
	} else {
		return FuncDeclKeepInfo{
			keep:   true,
			isTest: false,
		}
	}
}

func isTestFunction(fset *token.FileSet, funcDecl *ast.FuncDecl) bool {
	funcName := funcDecl.Name.Name
	if !strings.HasPrefix(funcName, "Test") {
		return false
	}

	testName := strings.TrimPrefix(funcName, "Test")
	if !ast.IsExported(testName) {
		return false
	}

	params := funcDecl.Type.Params.List
	if len(params) != 1 {
		return false
	}

	return typeToStr(fset, params[0].Type) == "*testing.T"
}

func typeToStr(fset *token.FileSet, ty ast.Expr) string {
	var buf bytes.Buffer
	err := printer.Fprint(&buf, fset, ty)
	if err != nil {
		return ""
	}
	return buf.String()
}

var filesToKeep = []string{
	"setup_test.go",
	"syscalls_amd64_test.go",
	"syscalls_arm64_test.go",
	"trace_pipe.go",
	"cmdwrapper.go",
	"schemas.go", // little hack, this works for ./schemas.go and ./schemas/schemas.go
}

func shouldKeepVerbatim(filePath string) bool {
	base := path.Base(filePath)
	for _, file := range filesToKeep {
		if file == base {
			return true
		}
	}
	return false
}
