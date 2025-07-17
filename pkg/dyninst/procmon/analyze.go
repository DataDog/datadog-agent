// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// TODO: At some future point we may want to find a build ID and use that to
// cache the properties of the binary.
//
// [0] https://opentelemetry.io/docs/specs/otel/profiles/mappings/#algorithm-for-processexecutablebuild_idhtlhash

// ContainerResolver is an interface that can be used to resolve the container
// context of a process.
type ContainerResolver interface {
	GetContainerContext(pid uint32) (containerutils.ContainerID, model.CGroupContext, string, error)
}

// analyzeProcess performs light analysis of the process and its binary
// to determine if it's interesting, and what its executable is.
func analyzeProcess(
	pid uint32,
	procfsRoot string,
	resolver ContainerResolver,
	executableAnalyzer executableAnalyzer,
) (processAnalysis, error) {
	maybeWrapErr := func(msg string, err error) error {
		if err == nil || os.IsNotExist(err) || errors.Is(err, syscall.ESRCH) {
			return nil
		}
		pid, msg, err := pid, msg, err
		return fmt.Errorf("analyzeProcess: pid %d: %s: %w", pid, msg, err)
	}

	exeLinkPath := path.Join(procfsRoot, strconv.Itoa(int(pid)), "exe")
	exePath, err := os.Readlink(exeLinkPath)
	if err != nil {
		return processAnalysis{}, maybeWrapErr(
			"failed to open exe link", err,
		)
	}

	exeFile, err := os.Open(exePath)
	if os.IsNotExist(err) || os.IsPermission(err) {
		// Try to open the exe under the proc root which can work when the
		// file exists inside a container.
		exePath = path.Join(
			procfsRoot, strconv.Itoa(int(pid)), "root",
			strings.TrimPrefix(exePath, "/"),
		)
		if log.ShouldLog(log.TraceLvl) {
			exePath := exePath
			log.Tracef(
				"exe for pid %d does not exist in root fs, trying under proc_root: %s",
				pid, exePath,
			)
		}
		var rootErr error
		exeFile, rootErr = os.Open(exePath)
		if rootErr != nil {
			err = errors.Join(err, rootErr)
		}
	}
	if err != nil {
		return processAnalysis{}, maybeWrapErr("failed to open exe", err)
	}
	defer exeFile.Close()

	statI, err := exeFile.Stat()
	if err != nil {
		return processAnalysis{}, maybeWrapErr("failed to stat exe", err)
	}
	statSys := statI.Sys()
	st, ok := statSys.(*syscall.Stat_t)
	if !ok {
		return processAnalysis{}, maybeWrapErr(
			"failed to cast stat", fmt.Errorf("got %T", statSys),
		)
	}
	fileKey := FileKey{
		FileHandle: FileHandle{
			Dev: uint64(st.Dev),
			Ino: st.Ino,
		},
		LastModified: st.Mtim,
	}

	interesting, known := executableAnalyzer.checkFileKeyCache(fileKey)
	if known && !interesting {
		return processAnalysis{}, nil
	}

	ddEnv, err := analyzeEnviron(int32(pid), procfsRoot)
	if err != nil {
		return processAnalysis{}, maybeWrapErr(
			"failed to analyze environ", err,
		)
	}
	if ddEnv.serviceName == "" || !ddEnv.diEnabled {
		log.Tracef(
			"process %d is not interesting: service name is %q, %s=%t",
			pid, ddEnv.serviceName, ddDynInstEnabledEnvVar, ddEnv.diEnabled,
		)
		return processAnalysis{interesting: false}, nil
	}

	isGo, err := executableAnalyzer.isInteresting(exeFile, fileKey)
	if !isGo || err != nil {
		return processAnalysis{}, maybeWrapErr(
			"failed to check if exe is interesting", err,
		)
	}

	containerInfo := analyzeContainerInfo(resolver, pid)

	return processAnalysis{
		service:     ddEnv.serviceName,
		exe:         Executable{Path: exePath, Key: fileKey},
		interesting: true,
		gitInfo: GitInfo{
			CommitSha:     ddEnv.gitCommitSha,
			RepositoryURL: ddEnv.gitRepositoryURL,
		},
		containerInfo: containerInfo,
	}, nil
}

type ddEnvVars struct {
	serviceName      string
	diEnabled        bool
	gitCommitSha     string
	gitRepositoryURL string
}

const ddServiceEnvVar = "DD_SERVICE"
const ddDynInstEnabledEnvVar = "DD_DYNAMIC_INSTRUMENTATION_ENABLED"
const ddGitCommitShaEnvVar = "DD_GIT_COMMIT_SHA"
const ddGitRepositoryURLEnvVar = "DD_GIT_REPOSITORY_URL"

var ddEnvVarBufPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 16<<10))
	},
}

func analyzeEnviron(pid int32, procfsRoot string) (ddEnvVars, error) {
	buf := ddEnvVarBufPool.Get().(*bytes.Buffer)
	defer ddEnvVarBufPool.Put(buf)
	defer buf.Reset()
	procEnv := path.Join(procfsRoot, strconv.Itoa(int(pid)), "environ")

	f, err := os.Open(procEnv)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ESRCH) {
			return ddEnvVars{}, nil
		}
		return ddEnvVars{}, fmt.Errorf(
			"failed to open proc env at %s: %w", procEnv, err,
		)
	}
	defer f.Close()
	_, err = buf.ReadFrom(f)
	if err != nil {
		return ddEnvVars{}, fmt.Errorf("failed to copy proc env: %w", err)
	}
	env := buf.Bytes()
	var ddEnv ddEnvVars
	for envVar, val := range envVars(env) {
		switch unsafe.String(unsafe.SliceData(envVar), len(envVar)) {
		case ddServiceEnvVar:
			ddEnv.serviceName = string(val)
		case ddDynInstEnabledEnvVar:
			ddEnv.diEnabled, _ = strconv.ParseBool(string(val))
		case ddGitCommitShaEnvVar:
			ddEnv.gitCommitSha = string(val)
		case ddGitRepositoryURLEnvVar:
			ddEnv.gitRepositoryURL = string(val)
		}
	}
	return ddEnv, nil
}

// envVars returns an iterator over the environment variables in the given
// procfs environment file.
func envVars(procEnviron []byte) iter.Seq2[[]byte, []byte] {
	return func(yield func(k, v []byte) bool) {
		cur := procEnviron
		for len(cur) > 0 {
			curVar := cur
			if idx := bytes.IndexByte(cur, 0); idx != -1 {
				curVar = cur[:idx]
				cur = cur[idx+1:]
			} else {
				cur = nil
			}
			eqIdx := bytes.IndexByte(curVar, '=')
			if eqIdx == -1 { // shouldn't happen, just skip it
				continue
			}
			if !yield(curVar[:eqIdx], curVar[eqIdx+1:]) {
				return
			}
		}
	}
}

// From https://github.com/torvalds/linux/blob/5859a2b1991101d6b978f3feb5325dad39421f29/include/linux/proc_ns.h#L41-L49
// Currently, host namespace inode number are hardcoded, which can be used to detect
// if we're running in host namespace or not (does not work when running in DinD)
const hostCgroupNamespaceInode = 0xEFFFFFFB

var containerResolverErrLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 10)

func analyzeContainerInfo(resolver ContainerResolver, pid uint32) ContainerInfo {
	containerID, cgroupContext, _, err := resolver.GetContainerContext(pid)
	if err != nil {
		if containerResolverErrLogLimiter.Allow() {
			log.Infof(
				"failed to get container context for pid %d: %v", pid, err,
			)
		}
		return ContainerInfo{}
	}
	// See https://github.com/DataDog/dd-trace-go/blob/0bf59472/internal/container_linux.go#L151-L155
	if containerID != "" {
		log.Tracef(
			"analyzeContainerInfo(%d): containerID: %s",
			pid, containerID,
		)
		return ContainerInfo{
			EntityID:    "ci-" + string(containerID),
			ContainerID: string(containerID),
		}
	}
	if cgroupContext.CGroupFile.Inode != hostCgroupNamespaceInode {
		return ContainerInfo{
			EntityID: fmt.Sprintf("in-%d", cgroupContext.CGroupFile.Inode),
		}
	}
	if log.ShouldLog(log.TraceLvl) {
		log.Tracef("analyzeContainerInfo(%d): no container info found", pid)
	}
	return ContainerInfo{}
}

var goSections = map[string]struct{}{
	".gosymtab":     {},
	".gopclntab":    {},
	".go.buildinfo": {},
}

// isGoElfBinaryWithDDTraceGo returns true if the given executable is an ELF
// file that contains go sections and debug info, and contains the relevant
// dd-trace-go symbols we need to instrument.
//
// In the future we may want to look and see here if there's a relevant
// version of the trace agent that we care about, but for now we leave
// that to later analysis of dwarf.
func isGoElfBinaryWithDDTraceGo(f *os.File) (bool, error) {
	elfFile, err := object.OpenMMappingElfFile(f.Name())
	if err != nil {
		log.Tracef("isGoElfBinary(%s): not an ELF file: %v", f.Name(), err)
		return false, nil
	}
	defer elfFile.Close() // no-op, but why not

	var symtabSection *safeelf.Section
	var hasDebugInfo, hasGoSections bool
	for _, section := range elfFile.Elf.Sections {
		if _, ok := goSections[section.Name]; ok {
			hasGoSections = true
		}
		if section.Name == ".debug_info" {
			hasDebugInfo = true
		}
		if section.Name == ".symtab" {
			symtabSection = section
		}
	}
	if log.ShouldLog(log.TraceLvl) {
		log.Tracef(
			"isGoElfBinary(%s): hasGoSections: %v, hasDebugInfo: %v, symtabSection: %v",
			f.Name(), hasGoSections, hasDebugInfo, symtabSection == nil,
		)
	}
	if !hasGoSections || !hasDebugInfo || symtabSection == nil {
		return false, nil
	}

	// This is a pretty rough heuristic but it's cheap. The way it works is to
	// find the string table for the symbol table and then scan it for the
	// strings corresponding to the symbols we might care about.
	symtabStringsSectionIdx := symtabSection.Link
	if symtabStringsSectionIdx >= uint32(len(elfFile.Elf.Sections)) {
		return false, nil
	}
	symtabStringsSection := elfFile.Elf.Sections[symtabStringsSectionIdx]
	symtabStrings, err := elfFile.MMap(symtabStringsSection, 0, symtabStringsSection.Size)
	if err != nil {
		return false, fmt.Errorf("failed to get symbols: %w", err)
	}
	defer symtabStrings.Close()
	return bytes.Contains(symtabStrings.Data, ddTraceSymbolSuffix), nil
}

var ddTraceSymbolSuffix = []byte("ddtrace/tracer.passProbeConfiguration")

// executableAnalyzer is an interface for analyzing executables to determine if
// they are interesting for dynamic instrumentation.
type executableAnalyzer interface {
	// checkFileKeyCache is used to check if the executable is interesting
	// without loading anything about the file. If known is false, the value
	// of interesting is meaningless.
	checkFileKeyCache(key FileKey) (interesting bool, known bool)
	// isInteresting is used to check if the executable is interesting.
	// It is called after checkFileKeyCache has returned true.
	//
	// Note that the file will be left in an unknown state after this call.
	isInteresting(f *os.File, key FileKey) (bool, error)
}

// baseExecutableAnalyzer implements executableAnalyzer without any caching.
type baseExecutableAnalyzer struct{}

func (a *baseExecutableAnalyzer) checkFileKeyCache(
	_ FileKey,
) (interesting bool, known bool) {
	return false, false
}

func (a *baseExecutableAnalyzer) isInteresting(
	f *os.File, _ FileKey,
) (bool, error) {
	return isGoElfBinaryWithDDTraceGo(f)
}

type fileKeyCacheExecutableAnalyzer struct {
	inner        executableAnalyzer
	fileKeyCache *lru.Cache[FileKey, bool]
}

func newFileKeyCacheExecutableAnalyzer(
	cacheSize int,
	inner executableAnalyzer,
) executableAnalyzer {
	return &fileKeyCacheExecutableAnalyzer{
		inner:        inner,
		fileKeyCache: mustNewLruCache[FileKey, bool](cacheSize),
	}
}

func (a *fileKeyCacheExecutableAnalyzer) checkFileKeyCache(
	key FileKey,
) (interesting bool, known bool) {
	return a.fileKeyCache.Get(key)
}

func (a *fileKeyCacheExecutableAnalyzer) isInteresting(
	f *os.File,
	key FileKey,
) (bool, error) {
	if interesting, ok := a.fileKeyCache.Get(key); ok {
		return interesting, nil
	}
	interesting, err := a.inner.isInteresting(f, key)
	if err != nil {
		return false, err
	}
	a.fileKeyCache.Add(key, interesting)
	return interesting, nil
}

type htlHashCacheExecutableAnalyzer struct {
	inner        executableAnalyzer
	htlHashCache *lru.Cache[string, bool]
}

func newHtlHashCacheExecutableAnalyzer(
	cacheSize int,
	inner executableAnalyzer,
) executableAnalyzer {
	return &htlHashCacheExecutableAnalyzer{
		inner:        inner,
		htlHashCache: mustNewLruCache[string, bool](cacheSize),
	}
}

func (a *htlHashCacheExecutableAnalyzer) isInteresting(
	f *os.File,
	key FileKey,
) (bool, error) {
	hash, err := computeHtlHash(f)
	if err != nil {
		return false, err
	}
	if interesting, ok := a.htlHashCache.Get(hash); ok {
		return interesting, nil
	}
	// Seek to the start of the file for the next call to isInteresting.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false, fmt.Errorf("failed to seek to start of file: %w", err)
	}
	interesting, err := a.inner.isInteresting(f, key)
	if err != nil {
		return false, err
	}
	a.htlHashCache.Add(hash, interesting)
	return interesting, nil
}

func (a *htlHashCacheExecutableAnalyzer) checkFileKeyCache(
	key FileKey,
) (interesting bool, known bool) {
	return a.inner.checkFileKeyCache(key)
}

func makeExecutableAnalyzer(cacheSize int) executableAnalyzer {
	if cacheSize <= 0 {
		return &baseExecutableAnalyzer{}
	}
	return newFileKeyCacheExecutableAnalyzer(
		cacheSize,
		newHtlHashCacheExecutableAnalyzer(cacheSize, &baseExecutableAnalyzer{}),
	)
}

// mustNewLruCache panics if the cache creation fails which will only happen
// if the size is non-positive.
func mustNewLruCache[K comparable, V any](size int) *lru.Cache[K, V] {
	c, err := lru.New[K, V](size)
	if err != nil {
		panic(err)
	}
	return c
}
