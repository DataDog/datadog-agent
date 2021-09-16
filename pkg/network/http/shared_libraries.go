// +build linux_bpf

package http

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/gopsutil/process/so"
)

const (
	libssl    = "libssl.so"
	libcrypto = "libcrypto.so"
)

var openSSLLibs = regexp.MustCompile(
	fmt.Sprintf("(%s|%s)", regexp.QuoteMeta(libssl), regexp.QuoteMeta(libcrypto)),
)

func findOpenSSLLibraries(procRoot string) []so.Library {
	// libraries will include all host-resolved openSSL library paths mapped into memory
	libraries := so.FindProc(procRoot, openSSLLibs)

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
