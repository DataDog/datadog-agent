package ebpf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/di/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

var (
	bpffs = "/sys/fs/bpf" //TODO: Check via `mount(2)`
)

var (
	globalTempDirPath string
	globalHeadersPath string
)

func SetupRingBufferAndHeaders() {
	tempDir, err := os.MkdirTemp("/tmp", "dd-go-di*")
	if err != nil {
		log.Info(err)
	}

	globalTempDirPath = tempDir

	err = setupRingbufferAndHeaders()
	if err != nil {
		log.Errorf("couldn't establish bpf ringbuffer: %s", err)
	}
}

func setupRingbufferAndHeaders() error {

	// Create temporary directory structure to unload headers into
	tempDir, err := os.MkdirTemp(globalTempDirPath, "run-*")
	if err != nil {
		return fmt.Errorf("couldn't create temp directory: %w", err)
	}

	headersPath, err := loadHeadersToTmpfs(tempDir)
	if err != nil {
		return fmt.Errorf("couldn't load headers to tmpfs: %w", err)
	}

	globalHeadersPath = headersPath

	objFilePath := filepath.Join(globalTempDirPath, "ringbuffer-go-di.bpf.o")
	cFlags := []string{
		"-O2",
		"-g",
		"--target=bpf",
		fmt.Sprintf("-I%s", globalHeadersPath),
		"-o",
		objFilePath,
	}

	// Read ringbuffer source file
	ringbufferSource, err := os.ReadFile(filepath.Join(headersPath, "ringbuffer.h"))
	if err != nil {
		return fmt.Errorf("could not read ringbuffer source code: %w", err)
	}

	ringbufferSourcePath := filepath.Join(globalTempDirPath, "ringbuffer-go-di.bpf.c")

	err = os.WriteFile(ringbufferSourcePath, ringbufferSource, 0644)
	if err != nil {
		return fmt.Errorf("could not write to ringbuffer source code file: %w", err)
	}

	// // Compile ringbuffer source file
	// cfg := ddebpf.NewConfig()
	// khOpts := kernel.HeaderOptions{
	// 	DownloadEnabled: cfg.EnableKernelHeaderDownload,
	// 	Dirs:            cfg.KernelHeadersDirs,
	// 	DownloadDir:     cfg.KernelHeadersDownloadDir,
	// 	AptConfigDir:    cfg.AptConfigDir,
	// 	YumReposDir:     cfg.YumReposDir,
	// 	ZypperReposDir:  cfg.ZypperReposDir,
	// }
	// kernelHeaders := kernel.GetKernelHeaders(khOpts, nil)
	// if len(kernelHeaders) == 0 {
	// 	return fmt.Errorf("unable to find kernel headers!")
	// }

	// err = compiler.CompileToObjectFile(ringbufferSourcePath, objFilePath, cFlags, []string{})
	err = clang(cFlags, ringbufferSourcePath, withStdout(os.Stdout))
	if err != nil {
		return fmt.Errorf("could not compile ringbuffer object: %w", err)
	}

	objFileReader, err := os.Open(objFilePath)
	if err != nil {
		return fmt.Errorf("could not open ringbuffer object: %w", err)
	}

	// Parse the compiled bpf object ELF file
	spec, err := ebpf.LoadCollectionSpecFromReader(objFileReader)
	if err != nil {
		return fmt.Errorf("could not create ringbuffer collection: %w", err)
	}
	spec.Maps["events"].Pinning = ebpf.PinByName

	// Load the ebpf object
	opts := ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{
			PinPath: bpffs,
			LoadPinOptions: ebpf.LoadPinOptions{
				ReadOnly: true,
			},
		},
	}
	_, err = ebpf.NewCollectionWithOptions(spec, opts)
	if err != nil {
		return fmt.Errorf("could not load ringbuffer collection: %w", err)
	}

	return nil
}

func AttachBPFUprobe(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) error {
	executable, err := link.OpenExecutable(procInfo.BinaryPath)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", err.Error())
		return fmt.Errorf("could not open proc executable for attaching bpf probe: %w", err)
	}

	bpfReader, err := os.Open(probe.InstrumentationInfo.BPFObjectFilePath)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", err.Error())
		return fmt.Errorf("could not open bpf executable for instrumenting bpf probe: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(bpfReader)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", err.Error())
		return fmt.Errorf("could not create bpf collection for probe %s: %w", probe.ID, err)
	}

	if probe.ID != ditypes.ConfigBPFProbeID {
		// config probe is special and should not be on the same ringbuffer
		// as the rest of regular events. Despite having the same "events" name,
		// not using the pinned map means the config program uses a different
		// ringbuffer.
		spec.Maps["events"].Pinning = ebpf.PinByName
	}

	// Load the ebpf object
	opts := ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{
			PinPath: bpffs,
		},
	}

	bpfObject, err := ebpf.NewCollectionWithOptions(spec, opts)
	if err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			log.Infof("Verifier error: %+v\n", ve)
		}
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", err.Error())
		return fmt.Errorf("could not load bpf collection for probe %s: %w", probe.ID, err)
	}

	procInfo.InstrumentationObjects[probe.ID] = bpfObject

	// Populate map used for zero'ing out regions of memory
	zeroValMap, ok := bpfObject.Maps["zeroval"]
	if !ok {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", "could not find bpf map for zero value")
		return fmt.Errorf("could not find bpf map for zero value in bpf object")
	}

	var zeroSlice []uint8 = make([]uint8, probe.InstrumentationInfo.InstrumentationOptions.ArgumentsMaxSize)
	var index uint32
	err = zeroValMap.Update(index, zeroSlice, 0)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", "could not find bpf map for zero value")
		return fmt.Errorf("could not use bpf map for zero value in bpf object")
	}

	// Attach BPF probe to function in executable
	bpfProgram, ok := bpfObject.Programs[probe.GetBPFFuncName()]
	if !ok {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", fmt.Sprintf("couldn't find bpf program for symbol %s", probe.FuncName))
		return fmt.Errorf("could not find bpf program for symbol %s", probe.FuncName)
	}

	link, err := executable.Uprobe(probe.FuncName, bpfProgram, &link.UprobeOptions{
		PID: int(procInfo.PID),
	})
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "UPROBE_FAILURE", err.Error())
		return fmt.Errorf("could not attach bpf program via uprobe: %w", err)
	}

	procInfo.SetUprobeLink(probe.ID, &link)
	diagnostics.Diagnostics.SetStatus(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, ditypes.StatusInstalled)

	return nil
}

// CompileBPFProgram compiles the code for a single probe associated with the process given by procInfo
func CompileBPFProgram(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) error {
	tempDirPath, err := os.MkdirTemp(globalTempDirPath, "run-*")
	if err != nil {
		return err
	}

	bpfSourceFile, err := os.CreateTemp(tempDirPath, "*.bpf.c")
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "COMPILE_ERROR", err.Error())
		return fmt.Errorf("couldn't create temp file for bpf code of probe %s: %s", probe.ID, err)
	}
	defer bpfSourceFile.Close()

	_, err = bpfSourceFile.WriteString(probe.InstrumentationInfo.BPFSourceCode)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "COMPILE_ERROR", err.Error())
		return fmt.Errorf("couldn't write to temp file for bpf code of probe %s: %s", probe.ID, err)
	}

	objFilePath := filepath.Join(tempDirPath, fmt.Sprintf("%d-%s-go-di.bpf.o", procInfo.PID, probe.ID))
	cFlags := []string{
		"-O2",
		"-g",
		"--target=bpf",
		fmt.Sprintf("-I%s", globalHeadersPath),
		"-o",
		objFilePath,
	}

	// cfg := ddebpf.NewConfig()
	// opts := kernel.HeaderOptions{
	// 	DownloadEnabled: cfg.EnableKernelHeaderDownload,
	// 	Dirs:            cfg.KernelHeadersDirs,
	// 	DownloadDir:     cfg.KernelHeadersDownloadDir,
	// 	AptConfigDir:    cfg.AptConfigDir,
	// 	YumReposDir:     cfg.YumReposDir,
	// 	ZypperReposDir:  cfg.ZypperReposDir,
	// }
	// kernelHeaders := kernel.GetKernelHeaders(opts, nil)
	// if len(kernelHeaders) == 0 {
	// 	return fmt.Errorf("unable to find kernel headers!!")
	// }

	// err = compiler.CompileToObjectFile(bpfSourceFile.Name(), objFilePath, cFlags, []string{})
	err = clang(cFlags, bpfSourceFile.Name(), withStdout(os.Stdout))
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "COMPILE_ERROR", err.Error())
		return fmt.Errorf("couldn't compile BPF object for probe %s: %s", probe.ID, err)
	}

	probe.InstrumentationInfo.BPFObjectFilePath = objFilePath

	return nil
}

var (
	clangBinPath = getClangPath()
)

const (
	compilationStepTimeout = 60 * time.Second
)

func clang(cflags []string, inputFile string, options ...func(*exec.Cmd)) error {
	var clangErr bytes.Buffer

	clangCtx, clangCancel := context.WithTimeout(context.Background(), compilationStepTimeout)
	defer clangCancel()

	cflags = append(cflags, "-c")
	cflags = append(cflags, inputFile)

	compileToBC := exec.CommandContext(clangCtx, clangBinPath, cflags...)
	for _, opt := range options {
		opt(compileToBC)
	}
	compileToBC.Stderr = &clangErr

	err := compileToBC.Run()
	if err != nil {
		var errMsg string
		if clangCtx.Err() == context.DeadlineExceeded {
			errMsg = "operation timed out"
		} else if len(clangErr.String()) > 0 {
			errMsg = clangErr.String()
		} else {
			errMsg = err.Error()
		}
		return fmt.Errorf("clang: %s", errMsg)
	}

	if len(clangErr.String()) > 0 {
		return fmt.Errorf("%s", clangErr.String())
	}
	return nil
}

func withStdout(out io.Writer) func(*exec.Cmd) {
	return func(c *exec.Cmd) {
		c.Stdout = out
	}
}

func getClangPath() string {
	clangPath := os.Getenv("CLANG_PATH")
	if clangPath == "" {
		clangPath = "/opt/datadog-agent/embedded/bin/clang-bpf"
	}
	return clangPath
}
