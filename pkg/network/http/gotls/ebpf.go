// +build linux_bpf

package gotls

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/twmb/murmur3"
)

const (
	probeDataMap        = "probe_data"
	readPartialCallsMap = "read_partial_calls"
	connTupByTlsConnMap = "conn_tup_by_tls_conn"

	// Copied from ../ebpf_main.go
	httpInFlightMap   = "http_in_flight"
	httpBatchesMap    = "http_batches"
	httpBatchStateMap = "http_batch_state"

	uidHashLength = 8
)

type state string

const (
	statePreinit     state = "preinit"
	stateInitFailed  state = "init-failed"
	stateInitialized state = "initialized"
)

const (
	writeProbe      = "uprobe/crypto/tls.(*Conn).Write"
	readProbe       = "uprobe/crypto/tls.(*Conn).Read"
	readReturnProbe = "uprobe/crypto/tls.(*Conn).Read/return"
	closeProbe      = "uprobe/crypto/tls.(*Conn).Close"
)

type GoTLSProgram struct {
	cfg      *config.Config
	bytecode bytecode.AssetReader
	state    state

	mu                 sync.Mutex
	knownBadPrograms   map[string]struct{}
	initializingTracer map[string]struct{}
	binaryPrograms     map[string]*goTLSBinaryProgram

	httpInFlightMap   *ebpf.Map
	httpBatchesMap    *ebpf.Map
	httpBatchStateMap *ebpf.Map
	sockByPidFdMap    *ebpf.Map
	rlimit            *unix.Rlimit
}

type goTLSBinaryProgram struct {
	binaryPath string
	manager    *manager.Manager
	constants  []manager.ConstantEditor
}

func NewGoTLSProgram(c *config.Config, sockFDMap *ebpf.Map) (*GoTLSProgram, error) {
	if !c.EnableHTTPSMonitoring {
		return nil, nil
	}

	var bytecode bytecode.AssetReader
	var err error
	if c.EnableRuntimeCompiler {
		bytecode, err = getRuntimeCompiledGoTLS(c)
		if err != nil {
			if !c.AllowPrecompiledFallback {
				return nil, fmt.Errorf("error compiling network go tls tracer: %s", err)
			}
			log.Warnf("error compiling network go tls tracer, falling back to pre-compiled: %s", err)
		}
	}

	if bytecode == nil {
		bytecode, err = netebpf.ReadGoTLSModule(c.BPFDir, c.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("could not read bpf module: %s", err)
		}
	}

	return &GoTLSProgram{
		cfg:                c,
		sockByPidFdMap:     sockFDMap,
		bytecode:           bytecode,
		state:              statePreinit,
		knownBadPrograms:   make(map[string]struct{}),
		initializingTracer: make(map[string]struct{}),
		binaryPrograms:     make(map[string]*goTLSBinaryProgram),
	}, nil
}

func (o *GoTLSProgram) ConfigureManager(m *manager.Manager) {
	if o == nil {
		return
	}
}

func (o *GoTLSProgram) ConfigureOptions(options *manager.Options) {
	if o == nil {
		return
	}

	// Inherit the same rlimit option
	o.rlimit = options.RLimit
}

func (o *GoTLSProgram) PostInit(m *manager.Manager) {
	if o == nil {
		return
	}

	// Grab the shared HTTP maps/perf maps from the central manager
	// to use in the sub-managers
	if err := o.getAllSharedMaps(m); err != nil {
		log.Warnf("failed to initialize go tls tracer: error getting shared map: %s", err)
		o.state = stateInitFailed
	}

	o.state = stateInitialized
}

func (o *GoTLSProgram) Start() {
	if o == nil {
		return
	}

	// TODO remove
	testProg := os.Getenv("GO_TLS_TEST")
	if testProg != "" {
		go func() {
			time.Sleep(2 * time.Second)
			err := o.TryInitBinaryTracer(testProg)
			if err != nil {
				log.Warnf("failed to initialize go tls binary tracer: %s", err)
			}
		}()
	}
}

func (o *GoTLSProgram) Stop() {
	if o == nil {
		return
	}

	// Stop all binary programs
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, p := range o.binaryPrograms {
		p.Stop()
	}
}

func (o *GoTLSProgram) getAllSharedMaps(m *manager.Manager) error {
	var err error

	o.httpInFlightMap, err = o.getSharedMap(m, httpInFlightMap)
	if err != nil {
		return err
	}

	o.httpBatchesMap, err = o.getSharedMap(m, httpBatchesMap)
	if err != nil {
		return err
	}

	o.httpBatchStateMap, err = o.getSharedMap(m, httpBatchStateMap)
	if err != nil {
		return err
	}

	return nil
}

func (o *GoTLSProgram) getSharedMap(m *manager.Manager, name string) (*ebpf.Map, error) {
	sharedMap, ok, err := m.GetMap(name)
	if err != nil {
		return nil, fmt.Errorf("could not get %q map from shared manager: %w", name, err)
	}
	if !ok {
		return nil, fmt.Errorf("could not get %q map from shared manager", name)
	}

	return sharedMap, nil
}

// shouldInitTracer determines if a path should have a new Go TLS tracer attached to it.
// It returns false if the path at the binary is already being traced
// (or is in the process of being attached to),
// or if the binary is known to be not a useful binary to trace.
// If it returns true, a value in o.initializingTracer[binaryPath] is inserted
// to reserve it and prevent multiple threads from attaching to the same binary.
func (o *GoTLSProgram) shouldInitTracer(binaryPath string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.binaryPrograms[binaryPath]; ok {
		return false
	}

	if _, ok := o.knownBadPrograms[binaryPath]; ok {
		return false
	}

	if _, ok := o.initializingTracer[binaryPath]; ok {
		return false
	}

	// Mark the path as being traced
	o.initializingTracer[binaryPath] = struct{}{}
	return true
}

func (o *GoTLSProgram) cancelTracerInit(binaryPath string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	delete(o.initializingTracer, binaryPath)
}

func (o *GoTLSProgram) cancelTracerInitAndMarkBad(binaryPath string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	delete(o.initializingTracer, binaryPath)
	o.knownBadPrograms[binaryPath] = struct{}{}
}

func (o *GoTLSProgram) storeBinaryProgram(binaryPath string, program *goTLSBinaryProgram) {
	o.mu.Lock()
	defer o.mu.Unlock()

	delete(o.initializingTracer, binaryPath)
	o.binaryPrograms[binaryPath] = program
}

func (o *GoTLSProgram) TryInitBinaryTracer(binaryPath string) error {
	if o.state != stateInitialized {
		return nil
	}

	// Ensure path is absolute and follow sym-links
	absolutePath, err := filepath.Abs(binaryPath)
	if err != nil {
		return fmt.Errorf("error resolving path %q as absolute: %w", binaryPath, err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return fmt.Errorf("error resolving path %q by following symbolic links: %w", absolutePath, err)
	}

	// Make sure the file isn't already being traced
	if !o.shouldInitTracer(resolvedPath) {
		return nil
	}
	binaryProgram, markAsKnownBad, err := o.tryInitBinaryTracer(binaryPath)
	if binaryProgram == nil {
		if markAsKnownBad {
			o.cancelTracerInitAndMarkBad(binaryPath)
		} else {
			o.cancelTracerInit(binaryPath)
		}

		// Err will only be non-nil if there was an actual error;
		// otherwise we just want to ignore the binary.
		return err
	}

	o.storeBinaryProgram(resolvedPath, binaryProgram)
	log.Debugf("attached Go TLS tracer to binary at %q", resolvedPath)
	return nil
}

func (o *GoTLSProgram) tryInitBinaryTracer(binaryPath string) (program *goTLSBinaryProgram, knownBad bool, e error) {
	// Open the file, and check if it is a ELF Go binary.
	// This part could probably use some help to make it more performant;
	// from what I understand, doing the full parse of the ELF binary
	// isn't cheap, and the Go version check isn't optimized for performance
	// (it involves scanning the 64 KiB of data
	// near the start of the file for the magic string).
	// While the work in parsing the ELF binary isn't discarded
	// if the binary ends up being a Go binary, it is discarded otherwise,
	// so there's a large chance that it is wasted work.
	f, err := os.Open(binaryPath)
	if err != nil {
		// Cannot resolve path, skip (return no error)
		knownBad = true
		return
	}
	defer f.Close()
	elfFile, err := elf.NewFile(f)
	if err != nil {
		// Binary is not ELF, skip (return no error)
		knownBad = true
		return
	}
	v, _, err := binversion.ReadElfBuildInfo(elfFile)
	if err != nil || v == "" {
		// Binary is not Go binary, skip (return no error)
		knownBad = true
		return
	}

	// Run the inspection process,
	// and try to get the TLS attachment arguments.
	attachmentArgs, err := InspectBinary(elfFile)
	if err != nil {
		// This might be due to the binary not using the tls functions
		// (if it doesn't make any HTTPS requests):
		// skip (return no error)
		knownBad = true
		return
	}

	// Create a new `goTLSBinaryProgram` and start it
	binaryProgram, err := o.startBinaryProgram(binaryPath, elfFile, attachmentArgs)
	if err != nil {
		knownBad = false
		e = fmt.Errorf("error starting new Go TLS tracer on binary at %q: %w", binaryPath, err)
		return
	}

	program = binaryProgram
	return
}

func (o *GoTLSProgram) startBinaryProgram(binaryPath string, elfFile *elf.File, attachmentArgs *attachmentArgs) (*goTLSBinaryProgram, error) {
	uid := getUID(binaryPath)

	readReturnProbes := []*manager.Probe{}
	for i, offset := range attachmentArgs.readReturnAddresses {
		readReturnProbes = append(readReturnProbes, &manager.Probe{
			BinaryPath: binaryPath,
			// Each return probe needs to have a unique uid value,
			// so add the index to the binary UID to make an overall UID.
			UID:          makeReturnUID(uid, i),
			Section:      readReturnProbe,
			UprobeOffset: offset,
		})
	}

	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: probeDataMap, Contents: []ebpf.MapKV{
				{
					Key:   int32(0),
					Value: attachmentArgs.probeData,
				},
			}},
			{Name: readPartialCallsMap},
			{Name: connTupByTlsConnMap},
			// Shared HTTP maps from the central manager
			// are added below using manager.Options.MapEditors
		},
		Probes: append([]*manager.Probe{
			{BinaryPath: binaryPath, UID: uid, Section: writeProbe, UprobeOffset: attachmentArgs.writeAddress},
			{BinaryPath: binaryPath, UID: uid, Section: readProbe, UprobeOffset: attachmentArgs.readAddress},
			{BinaryPath: binaryPath, UID: uid, Section: closeProbe, UprobeOffset: attachmentArgs.closeAddress},
		}, readReturnProbes...),
	}

	options := manager.Options{
		RLimit: o.rlimit,
		MapEditors: map[string]*ebpf.Map{
			httpInFlightMap:               o.httpInFlightMap,
			httpBatchesMap:                o.httpBatchesMap,
			httpBatchStateMap:             o.httpBatchStateMap,
			string(probes.SockByPidFDMap): o.sockByPidFdMap,
		},
		// TODO add offset guessing for tls base
		// TODO inject map spec editors
	}

	err := mgr.InitWithOptions(o.bytecode, options)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize manager: %w", err)
	}

	time.Sleep(5 * time.Second)
	err = mgr.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start manager: %w", err)
	}

	return &goTLSBinaryProgram{
		binaryPath: binaryPath,
		manager:    mgr,
	}, nil
}

func (o *goTLSBinaryProgram) Stop() {
	_ = o.manager.Stop(manager.CleanInternal)
}

func getUID(binaryPath string) string {
	sum := murmur3.StringSum64(binaryPath)
	hash := strconv.FormatInt(int64(sum), 16)
	if len(hash) >= uidHashLength {
		return hash[len(hash)-uidHashLength:]
	}

	return binaryPath
}

func makeReturnUID(uid string, returnNumber int) string {
	return fmt.Sprintf("%s_%x", uid, returnNumber)
}
