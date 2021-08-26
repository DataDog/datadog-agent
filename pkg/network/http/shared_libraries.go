// +build linux_bpf

package http

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/process/so"
)

type soRule struct {
	re           *regexp.Regexp
	registerCB   func(string) error
	unregisterCB func(string) error
}

// soWatcher provides a way to tie callback functions to the lifecycle of shared libraries
type soWatcher struct {
	procRoot   string
	all        *regexp.Regexp
	rules      []soRule
	registered map[string]func(string) error
}

func newSOWatcher(procRoot string, rules ...soRule) *soWatcher {
	allFilters := make([]string, len(rules))
	for i, r := range rules {
		allFilters[i] = r.re.String()
	}

	all := regexp.MustCompile(fmt.Sprintf("(%s)", strings.Join(allFilters, "|")))
	return &soWatcher{
		procRoot: procRoot,
		all:      all,
		rules:    rules,
	}
}

// Start consuming shared-library events
// TODO: add streaming option
func (w *soWatcher) Start() {
	sharedLibraries := getSharedLibraries(w.procRoot, w.all)
	w.sync(sharedLibraries)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			sharedLibraries := getSharedLibraries(w.procRoot, w.all)
			w.sync(sharedLibraries)
		}
	}()
}

func (w *soWatcher) sync(libraries []so.Library) {
	old := w.registered
	w.registered = make(map[string]func(string) error)

OuterLoop:
	for _, lib := range libraries {
		for _, r := range w.rules {
			path := lib.HostPath

			if _, registered := old[path]; registered {
				delete(old, path)
				continue OuterLoop
			}

			if r.re.MatchString(path) {
				w.register(path, r)
				continue OuterLoop
			}
		}
	}

	// Now we call the unregister callback for every shared library that is no longer mapped into memory
	for path, unregisterCB := range old {
		unregisterCB(path)
	}
}

func (w *soWatcher) register(libPath string, r soRule) {
	err := r.registerCB(libPath)
	if err != nil {
		log.Errorf("error activing probes for %s: %s", libPath, err)
		return
	}

	w.registered[libPath] = r.unregisterCB
}

func getSharedLibraries(procRoot string, filter *regexp.Regexp) []so.Library {
	// libraries will include all host-resolved library paths mapped into memory
	libraries := so.FindProc(procRoot, filter)

	// TODO: should we ensure all entries are unique in the `so` package instead?
	seen := make(map[string]struct{}, len(libraries))
	i := 0
	for j, lib := range libraries {
		if _, ok := seen[lib.HostPath]; !ok {
			libraries[i] = libraries[j]
			seen[lib.HostPath] = struct{}{}
			i++
		}
	}
	libraries = libraries[0:i]

	// we merge it with the library locations provided via the SSL_LIB_PATHS env variable
	if libsFromEnv := fromEnv(); len(libsFromEnv) > 0 {
		libraries = append(libraries, libsFromEnv...)
	}

	// prepend everything with the HOST_FS, which designates where the underlying
	// host file system is mounted. This is intended for internal testing only.
	if hostFS := os.Getenv("HOST_FS"); hostFS != "" {
		for i, lib := range libraries {
			libraries[i].HostPath = filepath.Join(hostFS, lib.HostPath)
		}
	}

	return libraries
}

// this is a temporary hack to inject a library that isn't yet mapped into memory
// you can specify a libssl path like:
// SSL_LIB_PATHS=/lib/x86_64-linux-gnu/libssl.so.1.1
// And add the optional libcrypto path as well:
// SSL_LIB_PATHS=/lib/x86_64-linux-gnu/libssl.so.1.1,/lib/x86_64-linux-gnu/libcrypto.so.1.1
func fromEnv() []so.Library {
	paths := os.Getenv("SSL_LIB_PATHS")
	if paths == "" {
		return nil
	}

	var libraries []so.Library
	for _, lib := range strings.Split(paths, ",") {
		libraries = append(libraries, so.Library{HostPath: lib})
	}

	return libraries
}
