// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symbol

import (
	"bytes"
	"errors"
	"fmt"
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

var sectionTypesToKeepForDynamicSymbols = []elf.SectionType{
	elf.SHT_GNU_HASH,
	elf.SHT_HASH,
	elf.SHT_REL,
	elf.SHT_RELA, //nolint:misspell
	elf.SHT_DYNSYM,
	elf.SHT_DYNAMIC,
	elf.SHT_GNU_VERDEF,
	elf.SHT_GNU_VERNEED,
	elf.SHT_GNU_VERSYM,
}

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
	// Data for non-file backed ELF files (eg. vdso)
	data []byte
	// Reader for file backed ELF files (nil for vdso)
	reader *os.File
	helper FileHelper
}

type SectionInfo struct {
	Name  string
	Flags elf.SectionFlag
}

func (e *elfWrapper) Close() error {
	if e.reader != nil {
		e.reader.Close()
	}
	return e.elfFile.Close()
}

func newElfWrapperFromVDSO(m *process.Mapping, pr process.Process) (ef *elfWrapper, err error) {
	// vdso is not backed by a file
	data := make([]byte, m.Length)
	_, err = pr.GetRemoteMemory().ReadAt(data, int64(m.Vaddr))
	if err != nil {
		return nil, fmt.Errorf("failed to read vdso memory for PID %d: %w", pr.PID(), err)
	}
	r := bytes.NewReader(data)
	elfFile, err := pfelf.NewFile(r, 0, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create elf file from vdso memory for PID %d: %w", pr.PID(), err)
	}
	return &elfWrapper{elfFile: elfFile, data: data, helper: &ProcessFileHelper{pid: pr.PID()}}, nil
}

func newElfWrapperFromMapping(m *process.Mapping, pr process.Process) (ef *elfWrapper, err error) {
	if m.IsVDSO() {
		return newElfWrapperFromVDSO(m, pr)
	}

	elfFile, err := pr.OpenELF(m.Path.String())
	if err != nil {
		return nil, fmt.Errorf("failed to open ELF file %s for PID %d: %w", m.Path.String(), pr.PID(), err)
	}
	defer func() {
		if err != nil {
			elfFile.Close()
		}
	}()

	r, err := pr.OpenMappingFile(m)
	if err != nil {
		return nil, fmt.Errorf("failed to open mapping file %s for PID %d: %w", m.Path.String(), pr.PID(), err)
	}
	defer func() {
		if err != nil {
			r.Close()
		}
	}()

	f, ok := r.(*os.File)
	if !ok {
		return nil, fmt.Errorf("failed to cast mapping file %s to *os.File for PID %d", m.Path.String(), pr.PID())
	}

	return newElfWrapper(elfFile, m.Path.String(), f, &ProcessFileHelper{pid: pr.PID()}, nil)
}

func newElfWrapperFromFile(filePath string, helper FileHelper) (ef *elfWrapper, err error) {
	procFilePath, err := helper.ExtractAsFile(filePath)
	if err != nil {
		return nil, err
	}

	elfFile, err := pfelf.Open(procFilePath)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			elfFile.Close()
		}
	}()

	f, err := os.Open(procFilePath)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			f.Close()
		}
	}()

	return newElfWrapper(elfFile, filePath, f, helper, nil)
}

func newElfWrapper(elfFile *pfelf.File, filePath string, reader *os.File, helper FileHelper, data []byte) (*elfWrapper, error) {
	err := elfFile.LoadSections()
	if err != nil {
		return nil, fmt.Errorf("failed to load sections for %s: %w", filePath, err)
	}
	return &elfWrapper{elfFile: elfFile, filePath: filePath, data: data, reader: reader, helper: helper}, nil
}

func (e *elfWrapper) GetPersistentPath() string {
	if e.reader != nil {
		return fmt.Sprintf("/proc/%v/fd/%v", selfPid, e.reader.Fd())
	}
	return ""
}

func (e *elfWrapper) ElfData() ([]byte, error) {
	if e.reader != nil {
		return nil, errors.New("elf data is not available for file backed ELF files")
	}
	return e.data, nil
}

func (e *elfWrapper) GetSectionsRequiredForDynamicSymbols() []SectionInfo {
	var sections []SectionInfo
	for _, section := range e.elfFile.Sections {
		if slices.Contains(sectionTypesToKeepForDynamicSymbols, section.Type) {
			sections = append(sections, SectionInfo{Name: section.Name, Flags: section.Flags})
		}
		if section.Type == elf.SHT_DYNSYM {
			// Add STRTAB (usually .dynstr) section linked to the DYNSYM (usually .dynsym) section if it exists
			if section.Link != 0 {
				linkSection := e.elfFile.Sections[section.Link]
				sections = append(sections, SectionInfo{Name: linkSection.Name, Flags: linkSection.Flags})
			}
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
