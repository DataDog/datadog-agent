// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symboluploader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path"
	"reflect"
	"runtime"
	"testing"

	"github.com/DataDog/jsonapi"
	"github.com/DataDog/zstd"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"
	"go.opentelemetry.io/ebpf-profiler/process"
	"go.opentelemetry.io/ebpf-profiler/remotememory"
	"go.opentelemetry.io/ebpf-profiler/reporter"

	"github.com/DataDog/datadog-agent/comp/host-profiler/symboluploader/symbol"
	elf "github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

var objcopyZstdSupport = CheckObjcopyZstdSupport(context.Background())

// dummyProcess implements pfelf.Process for testing purposes
type dummyProcess struct {
	pid libpf.PID
}

func (d *dummyProcess) PID() libpf.PID {
	return d.pid
}

func (d *dummyProcess) GetMachineData() process.MachineData {
	return process.MachineData{}
}

func (d *dummyProcess) GetMappings() ([]process.Mapping, uint32, error) {
	return nil, 0, errors.New("not implemented")
}

func (d *dummyProcess) GetThreads() ([]process.ThreadInfo, error) {
	return nil, errors.New("not implemented")
}

func (d *dummyProcess) GetRemoteMemory() remotememory.RemoteMemory {
	return remotememory.RemoteMemory{}
}

func (d *dummyProcess) GetMappingFileLastModified(_ *process.Mapping) int64 {
	return 0
}

func (d *dummyProcess) CalculateMappingFileID(m *process.Mapping) (libpf.FileID, error) {
	return libpf.FileIDFromExecutableFile(m.Path.String())
}

func (d *dummyProcess) OpenMappingFile(m *process.Mapping) (process.ReadAtCloser, error) {
	return os.Open(m.Path.String())
}

func (d *dummyProcess) OpenELF(name string) (*pfelf.File, error) {
	return pfelf.Open(name)
}

func (d *dummyProcess) Close() error {
	return nil
}

func (d *dummyProcess) GetExe() (libpf.String, error) {
	str, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", d.pid))
	if err != nil {
		return libpf.NullString, err
	}
	return libpf.Intern(str), nil
}

func (d *dummyProcess) GetProcessMeta(_ process.MetaConfig) process.ProcessMeta {
	return process.ProcessMeta{}
}

func newExecutableMetadata(t *testing.T, filePath, goBuildID string) *reporter.ExecutableMetadata {
	fileID, err := libpf.FileIDFromExecutableFile(filePath)
	require.NoError(t, err)
	mf := libpf.NewFrameMappingFile(libpf.FrameMappingFileData{
		FileID:     fileID,
		FileName:   libpf.Intern(path.Base(filePath)),
		GnuBuildID: "",
		GoBuildID:  goBuildID,
	})
	pr := &dummyProcess{pid: libpf.PID(os.Getpid())}
	m := &process.Mapping{
		Path: libpf.Intern(filePath),
	}
	return &reporter.ExecutableMetadata{
		MappingFile:       mf,
		Process:           pr,
		Mapping:           m,
		DebuglinkFileName: "",
	}
}

func findSymbol(f *elf.File, name string) *elf.Symbol {
	syms, err := f.Symbols()
	if err != nil {
		return nil
	}
	for _, sym := range syms {
		if sym.Name == name {
			return &sym
		}
	}
	return nil
}

func findDynamicSymbol(f *elf.File, name string) *elf.Symbol {
	syms, err := f.DynamicSymbols()
	if err != nil {
		return nil
	}
	for _, sym := range syms {
		if sym.Name == name {
			return &sym
		}
	}
	return nil
}

func checkGoPCLnTab(t *testing.T, f *elf.File, checkGoFunc bool) {
	section := f.Section(".gopclntab")
	require.NotNil(t, section)
	require.Equal(t, elf.SHT_PROGBITS, section.Type)
	data, err := section.Data()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(data), 16)

	var quantum byte
	switch runtime.GOARCH {
	case "amd64":
		quantum = 0x1
	case "arm64":
		quantum = 0x4
	}

	expectedHeader := []byte{0xf1, 0xff, 0xff, 0xff, 0x00, 0x00, quantum, 0x08}
	assert.Equal(t, expectedHeader, data[:8])

	if checkGoFunc {
		require.NotNil(t, findSymbol(f, "go:func.*"))
	}
}

func checkRequest(t *testing.T, req *http.Request, expectedSymbolSource symbol.Source, expectedGoPCLnTab bool, expectedContentEncoding string) {
	require.Equal(t, "POST", req.Method)
	if expectedContentEncoding != "" {
		require.Equal(t, expectedContentEncoding, req.Header.Get("Content-Encoding"))
	} else {
		require.NotContains(t, req.Header, "Content-Encoding")
	}

	_, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	require.NoError(t, err)
	boundary, ok := params["boundary"]
	require.True(t, ok)

	var reader io.ReadCloser
	if req.Header.Get("Content-Encoding") == "zstd" {
		reader = zstd.NewReader(req.Body)
		defer reader.Close()
	} else {
		reader = req.Body
	}

	mr := multipart.NewReader(reader, boundary)
	form, err := mr.ReadForm(1 << 20) // 1 MiB
	require.NoError(t, err)
	fhs, ok := form.File["elf_symbol_file"]
	require.True(t, ok)
	f, err := fhs[0].Open()
	require.NoError(t, err)
	defer f.Close()

	event, ok := form.File["event"]
	require.True(t, ok)
	e, err := event[0].Open()
	require.NoError(t, err)
	defer e.Close()

	// unmarshal json into map[string]string
	// check that the symbol source is correct
	result := make(map[string]string)
	err = json.NewDecoder(e).Decode(&result)
	require.NoError(t, err)
	require.Equal(t, expectedSymbolSource.String(), result["symbol_source"])

	elfFile, err := elf.NewFile(f)
	require.NoError(t, err)
	defer elfFile.Close()

	if expectedGoPCLnTab || expectedSymbolSource == symbol.SourceGoPCLnTab {
		checkGoPCLnTab(t, elfFile, true)
	}

	switch expectedSymbolSource {
	case symbol.SourceDynamicSymbolTable:
		require.NotNil(t, findDynamicSymbol(elfFile, "_cgo_panic"))
	case symbol.SourceSymbolTable:
		require.NotNil(t, findSymbol(elfFile, "main.main"))
	case symbol.SourceDebugInfo:
		require.True(t, hasDWARFData(elfFile))
	}
}

func hasDWARFData(elfFile *elf.File) bool {
	hasBuildID := false
	hasDebugStr := false
	for _, section := range elfFile.Sections {
		// NOBITS indicates that the section is actually empty, regardless of the size in the
		// section header.
		if section.Type == elf.SHT_NOBITS {
			continue
		}

		if section.Name == ".note.gnu.build-id" {
			hasBuildID = true
		}

		if section.Name == ".debug_str" || section.Name == ".zdebug_str" ||
			section.Name == ".debug_str.dwo" {
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

		if section.Name == ".debug_info" || section.Name == ".zdebug_info" {
			return true
		}
	}

	// Some alternate debug files only have a .debug_str section. For these we want to return true.
	// Use the absence of program headers and presence of a Build ID as heuristic to identify
	// alternate debug files.
	return len(elfFile.Progs) == 0 && hasBuildID && hasDebugStr
}

var testEndpoints = []string{"a.com", "b.com", "c.com", "d.com", "e.com"}

type uploaderOpts struct {
	uploadDynamicSymbols           bool
	uploadGoPCLnTab                bool
	disableDebugSectionCompression bool
}

func newTestUploader(ctx context.Context, opts uploaderOpts) (*DatadogSymbolUploader, error) {
	endpoints := make([]SymbolEndpoint, 0, len(testEndpoints))
	for _, e := range testEndpoints {
		endpoints = append(endpoints, SymbolEndpoint{
			Site:   e,
			APIKey: "api_key",
		})
	}

	cfg := &SymbolUploaderConfig{
		SymbolUploaderOptions: SymbolUploaderOptions{
			Enabled:              true,
			UploadDynamicSymbols: opts.uploadDynamicSymbols,
			UploadGoPCLnTab:      opts.uploadGoPCLnTab,
			SymbolQueryInterval:  0,
			SymbolEndpoints:      endpoints,
		},
		DisableDebugSectionCompression: opts.disableDebugSectionCompression,
	}
	return NewDatadogSymbolUploader(ctx, cfg)
}

type buildOptions struct {
	dynsym           bool
	symtab           bool
	debugInfos       bool
	corruptGoPCLnTab bool
}

func buildGo(t *testing.T, tmpDir, buildID string, opts buildOptions) string {
	f, err := os.CreateTemp(tmpDir, "helloworld")
	require.NoError(t, err)
	defer f.Close()

	exe := f.Name()
	args := []string{"build", "-o", exe}
	ldflags := "-ldflags=-buildid=" + buildID + " "
	if opts.dynsym {
		ldflags += "-linkmode=external "
	}

	args = append(args, ldflags, "./testdata/helloworld.go")
	cmd := exec.CommandContext(t.Context(), "go", args...) // #nosec G204
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to build test binary with `%v`: %s\n%s", cmd.Args, err, out)

	args = []string{"-R", ".note.gnu.build-id"}
	if opts.debugInfos && !opts.symtab {
		t.Errorf("Cannot have debug infos without symtab")
	}

	if !opts.debugInfos {
		if opts.symtab {
			args = append(args, "-g")
		} else {
			args = append(args, "-S")
		}
	}
	if opts.corruptGoPCLnTab {
		// Remove the pclntab section
		args = append(args, "-R", ".gopclntab")
	}
	args = append(args, exe)
	cmd = exec.CommandContext(t.Context(), "objcopy", args...)
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "failed to strip test binary with `%v`: %s\n%s", cmd.Args, err, out)

	return exe
}

func buildSymbolQueryResponse(t *testing.T, buildID string, symbolSource symbol.Source) string {
	var symbolFiles []SymbolFile
	if symbolSource != symbol.SourceNone {
		symbolFiles = []SymbolFile{
			{
				ID:           "1",
				BuildID:      buildID,
				SymbolSource: symbolSource.String(),
				BuildIDType:  "go_build_id",
			},
		}
	}
	r, err := jsonapi.Marshal(&symbolFiles)
	require.NoError(t, err)
	return string(r)
}

func registerResponders(t *testing.T, buildID string) []chan *http.Request {
	channels := make([]chan *http.Request, 0, len(testEndpoints))
	symbolSources := []symbol.Source{symbol.SourceNone, symbol.SourceDynamicSymbolTable, symbol.SourceSymbolTable, symbol.SourceGoPCLnTab, symbol.SourceDebugInfo}
	for i, e := range testEndpoints {
		c := make(chan *http.Request, 1)
		channels = append(channels, c)
		httpmock.RegisterResponder("POST", buildSymbolQueryURL(e),
			httpmock.NewStringResponder(200, buildSymbolQueryResponse(t, buildID, symbolSources[i])))
		httpmock.RegisterResponder("POST", buildSourcemapIntakeURL(e),
			func(req *http.Request) (*http.Response, error) {
				// Read request body before sending response otherwise client will receive the response before having sent all the data
				b, err := io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				req.Body = io.NopCloser(bytes.NewReader(b))
				c <- req
				return httpmock.NewStringResponse(200, ""), nil
			})
	}
	return channels
}

//nolint:tparallel
func TestSymbolUpload(t *testing.T) {
	t.Parallel()
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	slog.SetLogLoggerLevel(slog.LevelDebug)
	buildID := "some_go_build_id"
	channels := registerResponders(t, buildID)

	checkUploadsWithEncoding := func(t *testing.T, expectedSymbolSource symbol.Source, expectedGoPCLnTab bool, expectedUploads []bool, expectedEncoding string) {
		callCountInfo := httpmock.GetCallCountInfo()
		for i, e := range testEndpoints {
			assert.Equal(t, 1, callCountInfo["POST "+buildSymbolQueryURL(e)])
			if expectedUploads[i] {
				assert.Equal(t, 1, callCountInfo["POST "+buildSourcemapIntakeURL(e)])
				req := <-channels[i]
				checkRequest(t, req, expectedSymbolSource, expectedGoPCLnTab, expectedEncoding)
			} else {
				assert.Equal(t, 0, callCountInfo["POST "+buildSourcemapIntakeURL(e)])
			}
		}
	}

	checkUploads := func(t *testing.T, expectedSymbolSource symbol.Source, expectedGoPCLnTab bool, expectedUploads []bool) {
		expectedEncoding := ""
		if !objcopyZstdSupport {
			expectedEncoding = "zstd"
		}
		checkUploadsWithEncoding(t, expectedSymbolSource, expectedGoPCLnTab, expectedUploads, expectedEncoding)
	}

	goExeNoSymbols := buildGo(t, t.TempDir(), buildID, buildOptions{dynsym: false, symtab: false, debugInfos: false})
	goExeyDynsym := buildGo(t, t.TempDir(), buildID, buildOptions{dynsym: true, symtab: false, debugInfos: false})
	goExeSymtab := buildGo(t, t.TempDir(), buildID, buildOptions{dynsym: true, symtab: true, debugInfos: false})
	goExeDebugInfos := buildGo(t, t.TempDir(), buildID, buildOptions{dynsym: true, symtab: true, debugInfos: true})
	goExeyDynsymCorruptGoPCLnTab := buildGo(t, t.TempDir(), buildID, buildOptions{dynsym: true, symtab: false, debugInfos: false, corruptGoPCLnTab: true})
	goExeDebugInfosCorruptGoPCLnTab := buildGo(t, t.TempDir(), buildID, buildOptions{dynsym: true, symtab: true, debugInfos: true, corruptGoPCLnTab: true})

	t.Run("No symbol upload if no symbols", func(t *testing.T) {
		httpmock.ZeroCallCounters()
		uploader, err := newTestUploader(t.Context(), uploaderOpts{})
		require.NoError(t, err)
		uploader.Start(t.Context())

		uploader.UploadSymbols(newExecutableMetadata(t, goExeNoSymbols, buildID))
		uploader.Stop()

		assert.Equal(t, 0, httpmock.GetTotalCallCount())
	})

	t.Run("Upload if symtab", func(t *testing.T) {
		httpmock.ZeroCallCounters()
		uploader, err := newTestUploader(t.Context(), uploaderOpts{})
		require.NoError(t, err)
		uploader.Start(t.Context())

		uploader.UploadSymbols(newExecutableMetadata(t, goExeSymtab, buildID))
		uploader.Stop()

		checkUploads(t, symbol.SourceSymbolTable, false, []bool{true, true, false, false, false})
	})

	t.Run("Upload if debug info", func(t *testing.T) {
		httpmock.ZeroCallCounters()
		uploader, err := newTestUploader(t.Context(), uploaderOpts{})
		require.NoError(t, err)
		uploader.Start(t.Context())

		uploader.UploadSymbols(newExecutableMetadata(t, goExeDebugInfos, buildID))
		uploader.Stop()

		checkUploads(t, symbol.SourceDebugInfo, false, []bool{true, true, true, true, false})
	})

	t.Run("No upload if dynamic symbols", func(t *testing.T) {
		httpmock.ZeroCallCounters()
		uploader, err := newTestUploader(t.Context(), uploaderOpts{})
		require.NoError(t, err)
		uploader.Start(t.Context())

		uploader.UploadSymbols(newExecutableMetadata(t, goExeyDynsym, buildID))
		uploader.Stop()

		assert.Equal(t, 0, httpmock.GetTotalCallCount())
	})

	t.Run("Upload if dynamic symbols when enabled", func(t *testing.T) {
		httpmock.ZeroCallCounters()
		uploader, err := newTestUploader(t.Context(), uploaderOpts{uploadDynamicSymbols: true})
		require.NoError(t, err)
		uploader.Start(t.Context())

		uploader.UploadSymbols(newExecutableMetadata(t, goExeyDynsym, buildID))
		uploader.Stop()

		checkUploads(t, symbol.SourceDynamicSymbolTable, false, []bool{true, false, false, false, false})
	})

	t.Run("Upload pclntab when enabled", func(t *testing.T) {
		httpmock.ZeroCallCounters()
		uploader, err := newTestUploader(t.Context(), uploaderOpts{uploadGoPCLnTab: true})
		require.NoError(t, err)
		uploader.Start(t.Context())

		uploader.UploadSymbols(newExecutableMetadata(t, goExeNoSymbols, buildID))
		uploader.Stop()

		checkUploads(t, symbol.SourceGoPCLnTab, true, []bool{true, true, true, false, false})
	})

	t.Run("Upload debug infos if pclntab is corrupted", func(t *testing.T) {
		httpmock.ZeroCallCounters()
		uploader, err := newTestUploader(t.Context(), uploaderOpts{uploadGoPCLnTab: true})
		require.NoError(t, err)
		uploader.Start(t.Context())

		uploader.UploadSymbols(newExecutableMetadata(t, goExeDebugInfosCorruptGoPCLnTab, buildID))
		uploader.Stop()

		checkUploads(t, symbol.SourceDebugInfo, false, []bool{true, true, true, true, false})
	})

	t.Run("Upload dynamic symbols if pclntab is corrupted and only dyn sym when enabled", func(t *testing.T) {
		httpmock.ZeroCallCounters()
		uploader, err := newTestUploader(t.Context(), uploaderOpts{uploadDynamicSymbols: true, uploadGoPCLnTab: true})
		require.NoError(t, err)
		uploader.Start(t.Context())

		uploader.UploadSymbols(newExecutableMetadata(t, goExeyDynsymCorruptGoPCLnTab, buildID))
		uploader.Stop()

		checkUploads(t, symbol.SourceDynamicSymbolTable, false, []bool{true, false, false, false, false})
	})

	t.Run("No symbol upload if pclntab is corrupted and only dynsym", func(t *testing.T) {
		httpmock.ZeroCallCounters()
		uploader, err := newTestUploader(t.Context(), uploaderOpts{uploadGoPCLnTab: true})
		require.NoError(t, err)
		uploader.Start(t.Context())

		uploader.UploadSymbols(newExecutableMetadata(t, goExeyDynsymCorruptGoPCLnTab, buildID))
		uploader.Stop()

		checkUploads(t, symbol.SourceNone, false, []bool{false, false, false, false, false})
	})

	t.Run("Upload compressed request when debug section compression is disabled", func(t *testing.T) {
		httpmock.ZeroCallCounters()
		uploader, err := newTestUploader(t.Context(), uploaderOpts{disableDebugSectionCompression: true})
		require.NoError(t, err)
		uploader.Start(t.Context())

		uploader.UploadSymbols(newExecutableMetadata(t, goExeDebugInfos, buildID))
		uploader.Stop()

		checkUploadsWithEncoding(t, symbol.SourceDebugInfo, false, []bool{true, true, true, true, false}, "zstd")
	})
}

// Verify that the symbol uploader transport has the same exported fields as http.DefaultTransport
// when HTTP/2 is disabled
func TestTransport(t *testing.T) {
	cfg := &SymbolUploaderConfig{
		SymbolUploaderOptions: SymbolUploaderOptions{
			Enabled:         true,
			UseHTTP2:        false, // This forces creation of custom transport
			SymbolEndpoints: []SymbolEndpoint{{Site: "test.com", APIKey: "key"}},
		},
		Version: "test",
	}

	uploader, err := NewDatadogSymbolUploader(t.Context(), cfg)
	require.NoError(t, err)

	customTransport, ok := uploader.client.Transport.(*http.Transport)
	require.True(t, ok, "Expected custom transport to be *http.Transport")

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok, "Expected http.DefaultTransport to be *http.Transport")

	// Use reflection to compare all exported fields
	customValue := reflect.ValueOf(customTransport).Elem()
	defaultValue := reflect.ValueOf(defaultTransport).Elem()
	customType := customValue.Type()

	for i := range customType.NumField() {
		field := customType.Field(i)
		if !field.IsExported() {
			continue
		}

		customFieldValue := customValue.Field(i)
		defaultFieldValue := defaultValue.Field(i)

		// Special handling for intentionally different fields
		switch field.Name {
		case "TLSNextProto":
			// TLSNextProto should be an empty map in custom transport to disable HTTP/2
			assert.NotNil(t, customFieldValue.Interface(), "TLSNextProto should not be nil")
			assert.Equal(t, 0, customFieldValue.Len(), "TLSNextProto should be empty map")
		case "ForceAttemptHTTP2":
			// ForceAttemptHTTP2 should be false in custom transport to disable HTTP/2
			assert.False(t, customFieldValue.Bool(), "ForceAttemptHTTP2 should be false to disable HTTP/2")
		case "TLSClientConfig":
			// TLSClientConfig should be nil in custom transport
			// The default transport might have a non-nil value if the default transport
			// was used in another test (for example through the default http client)
			assert.Nil(t, customFieldValue.Interface(), "TLSClientConfig should be nil")
		default:
			if customFieldValue.Kind() == reflect.Func {
				assert.Equal(t, defaultFieldValue.Pointer(), customFieldValue.Pointer(), "Function field %s should match", field.Name)
				continue
			}
			if !customFieldValue.CanInterface() || !defaultFieldValue.CanInterface() {
				assert.Failf(t, "Cannot compare field %s", field.Name)
				continue
			}
			assert.Equal(t, defaultFieldValue.Interface(), customFieldValue.Interface(),
				"Field %s should match default transport", field.Name)
		}
	}
}
