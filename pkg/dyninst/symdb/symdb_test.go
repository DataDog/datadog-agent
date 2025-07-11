package symdb_test

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
	"github.com/stretchr/testify/require"
)

func TestSymDB(t *testing.T) {
	cfgs, err := testprogs.GetCommonConfigs()
	require.NoError(t, err)
	for _, cfg := range cfgs {
		binaryPath, err := testprogs.GetBinary("simple", cfg)
		require.NoError(t, err)
		elf, err := safeelf.Open(binaryPath)
		require.NoError(t, err)
		file, err := object.NewElfObject(elf)
		require.NoError(t, err)
		// !!!
		//obj, err := object.NewElfObject(elf)
		//require.NoError(t, err)
		//dwarfData, err := obj.DWARF()
		//require.NoError(t, err)
		//
		//lineTableSect := elf.Section(".gopclntab")
		//textSect := elf.Section(".text")
		//if lineTableSect == nil || textSect == nil {
		//	t.Fatalf("missing required sections in %s", binaryPath)
		//}
		//
		//lineTableData, err := lineTableSect.Data()
		//if err != nil {
		//	t.Fatalf("read .gopclntab: %s", err)
		//}
		//
		//textAddr := elf.Section(".text").Addr
		//lineTable := gosym.NewLineTable(lineTableData, textAddr)
		//symTable, err := gosym.NewTable([]byte{}, lineTable)
		//if err != nil {
		//	t.Fatalf("failed to parse .gosymtab: %s", err)
		//}
		//
		//loclistReader, err := obj.LoclistReader()
		//if err != nil {
		//	t.Fatalf("failed to create loclist reader: %s", err)
		//}
		//symBuilder := symdb.NewSymDBBuilder(dwarfData, symTable, loclistReader, 8)
		symBuilder, err := symdb.NewSymDBBuilder(file)
		require.NoError(t, err)
		symbols, err := symBuilder.ExtractSymbols()
		require.NoError(t, err, "failed to extract symbols from %s", binaryPath)
		require.NotEmpty(t, symbols.Packages)

		pkg, ok := findPackage(symbols, "main")
		require.Truef(t, ok, "package 'main' not found in %s", binaryPath)
		fn, ok := findFunction(pkg, "stringArg")
		require.Truef(t, ok, "function 'stringArg' not found in package 'main' in %s", binaryPath)
		v, ok := findVariable(fn.Scope, "s")
		require.Truef(t, ok, "variable 's' not found in function 'stringArg' in package 'main' in %s", binaryPath)
		require.True(t, v.FunctionArgument)
		require.NotZero(t, v.DeclLine)
		require.NotEmpty(t, v.AvailableLineRanges)
	}
}

func findPackage(s symdb.Symbols, pkgName string) (symdb.Package, bool) {
	for _, pkg := range s.Packages {
		if pkg.Name == pkgName {
			return pkg, true
		}
	}
	return symdb.Package{}, false
}

func findFunction(pkg symdb.Package, fnName string) (symdb.Function, bool) {
	for _, fn := range pkg.Functions {
		if fn.Name == fnName {
			return fn, true
		}
	}
	return symdb.Function{}, false
}

func findType(pkg symdb.Package, typeName string) (symdb.Type, bool) {
	for _, typ := range pkg.Types {
		if typ.Name == typeName {
			return typ, true
		}
	}
	return symdb.Type{}, false
}

func findMethod(typ symdb.Type, methodName string) (symdb.Function, bool) {
	for _, method := range typ.Methods {
		if method.Name == methodName {
			return method, true
		}
	}
	return symdb.Function{}, false
}

func findVariable(scope symdb.Scope, varName string) (symdb.Variable, bool) {
	for _, variable := range scope.Variables {
		if variable.Name == varName {
			return variable, true
		}
	}
	return symdb.Variable{}, false
}
