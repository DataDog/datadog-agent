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
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

func main() {
	var (
		input   string
		output  string
		pkgName string
	)

	flag.StringVar(&input, "input", "", "Go tests folder")
	flag.StringVar(&output, "output", "", "Go embedded tests output folder")
	flag.StringVar(&pkgName, "pkg_name", "embeddedtests", "Output package name")
	flag.Parse()

	if input == "" || output == "" {
		panic(errors.New("please provide an input and output directory"))
	}

	os.RemoveAll(output)

	fset := token.NewFileSet()
	totalTests := make([]string, 0)

	if err := filepath.Walk(input, func(filepath string, info os.FileInfo, err error) error {
		opts := newEmbedFileOptions(filepath, input, output)
		if shouldKeepVerbatim(filepath) {
			if err := embedVerbatimFile(opts, pkgName); err != nil {
				return err
			}
		} else if strings.HasSuffix(path.Base(filepath), "_test.go") {
			tests, err := embedTestFile(fset, opts, pkgName)
			if err != nil {
				return err
			}
			totalTests = append(totalTests, tests...)
		}
		return nil
	}); err != nil {
		panic(err)
	}

	if err := finishOutputDir(input, output, pkgName, totalTests); err != nil {
		panic(err)
	}
}

type driverInfo struct {
	PkgName   string
	TestNames []string
}

func finishOutputDir(inputDir, outputDir string, pkgName string, testNames []string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	info := &driverInfo{
		PkgName:   pkgName,
		TestNames: testNames,
	}
	if err := writeTemplateFile(inputDir, outputDir, "driver.go", "pkg/security/tests/embed_generator/driver.go.tmpl", "test-driver", info); err != nil {
		return err
	}
	return writeTemplateFile(inputDir, outputDir, "driver_unsupported.go", "pkg/security/tests/embed_generator/driver_unsupported.go.tmpl", "test-driver-unsupported", info)
}

func writeTemplateFile(inputDir, outputDir, outputPath, templatePath, templateName string, info *driverInfo) error {
	templateCode, err := ioutil.ReadFile(templatePath)
	if err != nil {
		return err
	}

	tmpl, err := template.New(templateName).Parse(string(templateCode))
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, info); err != nil {
		return err
	}

	return writeOutputFile(inputDir, outputDir, path.Join(outputDir, outputPath), buf.Bytes(), true)
}

type embedFileOptions struct {
	inputDir   string
	inputPath  string
	outputDir  string
	outputPath string
}

func newEmbedFileOptions(inputPath, inputDir, outputDir string) embedFileOptions {
	fileName := strings.TrimPrefix(inputPath, inputDir)
	outputPath := path.Join(outputDir, fileName)

	if strings.HasSuffix(outputPath, "_test.go") {
		outputPath = strings.TrimSuffix(outputPath, "_test.go") + ".go"
	}

	return embedFileOptions{
		inputDir:   inputDir,
		inputPath:  inputPath,
		outputDir:  outputDir,
		outputPath: outputPath,
	}
}

var packageNameReplacer = regexp.MustCompile(`(?m)^package tests\s*$`)

func embedVerbatimFile(opts embedFileOptions, pkgName string) error {
	content, err := ioutil.ReadFile(opts.inputPath)
	if err != nil {
		return err
	}

	content = packageNameReplacer.ReplaceAll(content, []byte(fmt.Sprintf("package %v\n", pkgName)))
	return writeOutputFile(opts.inputDir, opts.outputDir, opts.outputPath, content, false)
}

func embedTestFile(fset *token.FileSet, opts embedFileOptions, pkgName string) ([]string, error) {
	node, err := parser.ParseFile(fset, opts.inputPath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	tests := make([]string, 0)
	shouldExportEmbedFile := false
	resDecls := make([]ast.Decl, 0, len(node.Decls))
	for _, decl := range node.Decls {
		keep := false
		funcDecl, ok := decl.(*ast.FuncDecl)
		if ok {
			info := getFuncDeclKeepInfo(fset, funcDecl)
			keep = info.keep
			if info.isTest {
				shouldExportEmbedFile = true
				tests = append(tests, funcDecl.Name.Name)
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

		var buf bytes.Buffer
		if err := printer.Fprint(&buf, fset, node); err != nil {
			return nil, err
		}

		if err := writeOutputFile(opts.inputDir, opts.outputDir, opts.outputPath, buf.Bytes(), false); err != nil {
			return nil, err
		}
		return tests, nil
	}

	return nil, nil
}

var embedCmdRegex = regexp.MustCompile(`//\s*test:embed`)

type funcDeclKeepInfo struct {
	keep   bool
	isTest bool
}

func getFuncDeclKeepInfo(fset *token.FileSet, funcDecl *ast.FuncDecl) funcDeclKeepInfo {
	if isTestFunction(fset, funcDecl) {
		if funcDecl.Doc != nil {
			for _, comment := range funcDecl.Doc.List {
				if embedCmdRegex.MatchString(comment.Text) {
					return funcDeclKeepInfo{
						keep:   true,
						isTest: true,
					}
				}
			}
		}
		return funcDeclKeepInfo{
			keep:   false,
			isTest: false,
		}
	}
	return funcDeclKeepInfo{
		keep:   true,
		isTest: false,
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

const editProtector = "// Code generated - DO NOT EDIT.\n"

func writeOutputFile(inputDir, outputDir, outputPath string, content []byte, skipBuildConstraintConversion bool) error {
	// create all needed subdirectories
	dirPath := path.Dir(outputPath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}

	tmp, err := ioutil.TempFile(dirPath, "temp-pre-fmt")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	inputPackage := path.Join("github.com/DataDog/datadog-agent", inputDir)
	outputPackage := path.Join("github.com/DataDog/datadog-agent", outputDir)

	prefixedContent := append([]byte(editProtector), content...)
	prefixedContent = bytes.Replace(prefixedContent, []byte(inputPackage), []byte(outputPackage), -1)
	var finalContent []byte
	if skipBuildConstraintConversion {
		finalContent = prefixedContent
	} else {
		buildEditedContent, err := convertBuildTags(string(prefixedContent))
		if err != nil {
			return err
		}
		finalContent = []byte(buildEditedContent)
	}

	if _, err := tmp.Write(finalContent); err != nil {
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	cmd := exec.Command("go", "run", "golang.org/x/tools/cmd/goimports", "-w", tmp.Name())
	if err := cmd.Run(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), outputPath)
}
