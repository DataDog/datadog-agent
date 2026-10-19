// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symbol

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"

	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"
	"go.opentelemetry.io/ebpf-profiler/process"

	elf "github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

const buildIDSectionName = ".note.gnu.build-id"

var debugStrSectionNames = []string{".debug_str", ".zdebug_str", ".debug_str.dwo"}
var debugInfoSectionNames = []string{".debug_info", ".zdebug_info"}
var globalDebugDirectories = []string{"/usr/lib/debug"}

var selfPid = os.Getpid()

type FileHelper interface {
	ExtractAsFile(string) (string, error)
}

type ProcessFileHelper struct {
	pid libpf.PID
}

func (p *ProcessFileHelper) ExtractAsFile(filePath string) (string, error) {
	return path.Join("/proc", strconv.Itoa(int(p.pid)), "root", filePath), nil
}

type elfWrapper struct {
	elfFile  *pfelf.File
	filePath string
	helper   FileHelper
}

type SectionInfo struct {
	Name  string
	Flags elf.SectionFlag
}

func (e *elfWrapper) Close() error {
	// elfFile owns the underlying reader and closes it
	return e.elfFile.Close()
}

func newElfWrapperFromMapping(m *process.RawMapping, pr process.Process) (ef *elfWrapper, err error) {
	mf, err := pr.OpenMappingFile(m)
	if err != nil {
		return nil, fmt.Errorf("failed to open mapping file %s for PID %d: %w", m.Path, pr.PID(), err)
	}

	// elfFile takes ownership of mf
	elfFile, err := pfelf.NewFileFromFS(mf)
	if err != nil {
		return nil, fmt.Errorf("failed to open ELF file %s for PID %d: %w", m.Path, pr.PID(), err)
	}

	ef, err = newElfWrapper(elfFile, m.Path, &ProcessFileHelper{pid: pr.PID()})
	if err != nil {
		_ = elfFile.Close()
	}
	return ef, err
}

func newElfWrapperFromFile(filePath string, helper FileHelper) (ef *elfWrapper, err error) {
	procFilePath, err := helper.ExtractAsFile(filePath)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(procFilePath)
	if err != nil {
		return nil, err
	}
	// elfFile takes ownership of f
	elfFile, err := pfelf.NewFileFromFS(f)
	if err != nil {
		return nil, err
	}

	ef, err = newElfWrapper(elfFile, filePath, helper)
	if err != nil {
		_ = elfFile.Close()
	}
	return ef, err
}

func newElfWrapper(elfFile *pfelf.File, filePath string, helper FileHelper) (*elfWrapper, error) {
	err := elfFile.LoadSections()
	if err != nil {
		return nil, fmt.Errorf("failed to load sections for %s: %w", filePath, err)
	}
	return &elfWrapper{elfFile: elfFile, filePath: filePath, helper: helper}, nil
}

// GetPersistentPath returns a path to the ELF file that remains valid while the
// wrapper is open, or "" if the ELF file is not backed by a file (eg. vdso).
func (e *elfWrapper) GetPersistentPath() string {
	if f, ok := e.elfFile.Underlying().(*os.File); ok {
		return fmt.Sprintf("/proc/%v/fd/%v", selfPid, f.Fd())
	}
	return ""
}

// GetSize returns the size of the ELF file.
func (e *elfWrapper) GetSize() (int64, error) {
	f, ok := e.elfFile.Underlying().(fs.File)
	if !ok {
		return 0, errors.New("underlying ELF reader is not a fs.File")
	}
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// ElfData returns the full content of the ELF file.
func (e *elfWrapper) ElfData() ([]byte, error) {
	size, err := e.GetSize()
	if err != nil {
		return nil, err
	}
	return io.ReadAll(io.NewSectionReader(e.elfFile.Underlying(), 0, size))
}

func (e *elfWrapper) GetSectionsRequiredForDynamicSymbols() []SectionInfo {
	var sections []SectionInfo
	for _, section := range e.elfFile.Sections {
		if section.Type != elf.SHT_DYNSYM {
			continue
		}
		sections = append(sections, SectionInfo{Name: section.Name, Flags: section.Flags})

		// Symbol names are in the linked STRTAB (usually .dynstr)
		if section.Link != 0 && int(section.Link) < len(e.elfFile.Sections) {
			linkSection := e.elfFile.Sections[section.Link]
			sections = append(sections, SectionInfo{Name: linkSection.Name, Flags: linkSection.Flags})
		}
	}

	return sections
}

func (e *elfWrapper) openELF(filePath string) (*elfWrapper, error) {
	return newElfWrapperFromFile(filePath, e.helper)
}

func (e *elfWrapper) symbolSource() Source {
	if HasDWARFData(e.elfFile) {
		return SourceDebugInfo
	}

	if e.elfFile.Section(".symtab") != nil {
		return SourceSymbolTable
	}

	if e.elfFile.Section(".dynsym") != nil {
		return SourceDynamicSymbolTable
	}

	return SourceNone
}

// findSeparateSymbolsWithDebugInfo attempts to find a separate symbol source for the elf file,
// following the same order as GDB
// https://sourceware.org/gdb/current/onlinedocs/gdb.html/Separate-Debug-Files.html
func (e *elfWrapper) findSeparateSymbolsWithDebugInfo() *elfWrapper {
	slog.Debug("No debug symbols found in file", slog.String("path", e.filePath))

	// First, check based on the GNU build ID
	debugElf := e.findDebugSymbolsWithBuildID()
	if debugElf != nil {
		if HasDWARFData(debugElf.elfFile) {
			return debugElf
		}
		debugElf.Close()
		slog.Debug("No debug symbols found in buildID link file", slog.String("path", debugElf.filePath))
	}

	// Then, check based on the debug link
	debugElf = e.findDebugSymbolsWithDebugLink()
	if debugElf != nil {
		if HasDWARFData(debugElf.elfFile) {
			return debugElf
		}
		slog.Debug("No debug symbols found in debug link file", slog.String("path", debugElf.filePath))
		debugElf.Close()
	}

	return nil
}

func (e *elfWrapper) findDebugSymbolsWithBuildID() *elfWrapper {
	buildID, err := e.elfFile.GetBuildID()
	if err != nil || len(buildID) < 2 {
		slog.Debug("Failed to get build ID for file", slog.String("path", e.filePath), slog.String("error", err.Error()))
		return nil
	}

	// Try to find the debug file
	for _, dir := range globalDebugDirectories {
		debugFile := filepath.Join(dir, ".build-id", buildID[:2], buildID[2:]+".debug")
		debugELF, err := e.openELF(debugFile)
		if err != nil {
			continue
		}
		debugBuildID, err := debugELF.elfFile.GetBuildID()
		if err != nil || buildID != debugBuildID {
			debugELF.Close()
			continue
		}
		return debugELF
	}
	return nil
}

func (e *elfWrapper) findDebugSymbolsWithDebugLink() *elfWrapper {
	linkName, linkCRC32, err := e.elfFile.GetDebugLink()
	if err != nil {
		return nil
	}

	// Try to find the debug file
	executablePath := filepath.Dir(e.filePath)

	debugDirectories := []string{
		executablePath,
		filepath.Join(executablePath, ".debug"),
	}
	for _, dir := range globalDebugDirectories {
		debugDirectories = append(debugDirectories,
			filepath.Join(dir, executablePath))
	}

	for _, debugPath := range debugDirectories {
		debugFile := filepath.Join(debugPath, executablePath, linkName)
		debugELF, err := e.openELF(debugFile)
		if err != nil {
			continue
		}
		fileCRC32, err := debugELF.elfFile.CRC32()
		if err != nil || fileCRC32 != linkCRC32 {
			debugELF.Close()
			continue
		}
		return debugELF
	}
	return nil
}

// HasDWARFData is a copy of pfelf.HasDWARFData, but for the libpf.File interface.
func HasDWARFData(f *pfelf.File) bool {
	hasBuildID := false
	hasDebugStr := false
	for _, section := range f.Sections {
		// NOBITS indicates that the section is actually empty, regardless of the size in the
		// section header.
		if section.Type == elf.SHT_NOBITS {
			continue
		}

		if section.Name == buildIDSectionName {
			hasBuildID = true
		}

		if slices.Contains(debugStrSectionNames, section.Name) {
			hasDebugStr = section.Size > 0
		}

		// Some files have suspicious near-empty, partially stripped sections; consider them as not
		// having DWARF data.
		// The simplest binary gcc 10 can generate ("return 0") has >= 48 bytes for each section.
		// Let's not worry about executables that may not verify this, as they would not be of
		// interest to us.
		if section.Size < 32 {
			continue
		}

		if slices.Contains(debugInfoSectionNames, section.Name) {
			return true
		}
	}

	// Some alternate debug files only have a .debug_str section. For these we want to return true.
	// Use the absence of program headers and presence of a Build ID as heuristic to identify
	// alternate debug files.
	return len(f.Progs) == 0 && hasBuildID && hasDebugStr
}
