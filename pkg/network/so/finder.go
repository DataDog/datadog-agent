package so

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/common"
)

type finder struct {
	procRoot     string
	pathResolver *pathResolver
	buffer       *bufio.Reader
}

func newFinder(procRoot string) *finder {
	buffer := bufio.NewReader(nil)
	return &finder{
		procRoot:     procRoot,
		pathResolver: newPathResolver(procRoot, buffer),
		buffer:       buffer,
	}
}

func (f *finder) Find(filter *regexp.Regexp) []string {
	seen := make(map[key]struct{})
	result := common.NewStringSet()
	iteratePIDS(f.procRoot, func(pidPath string, info os.FileInfo, mntNS ns) {
		libs := getSharedLibraries(pidPath, f.buffer, filter)

		// If we have already seen (mntNS, lib) we skip the path resolution process
		libs = excludeAlreadySeen(seen, mntNS, libs)

		if len(libs) == 0 {
			return
		}

		mountInfo := getMountInfo(pidPath, f.buffer)
		if mountInfo == nil {
			return
		}

		for _, lib := range libs {
			if hostPath := f.pathResolver.Resolve(lib, mountInfo); hostPath != "" {
				result.Add(hostPath)
			}
		}
	})
	return result.GetAll()
}

func iteratePIDS(procRoot string, fn callback) {
	w := newWalker(procRoot, fn)
	filepath.Walk(procRoot, filepath.WalkFunc(w.walk))
}

// key is used to keep track of which libraries we've seen
type key struct {
	ns   ns
	name string
}

func excludeAlreadySeen(seen map[key]struct{}, ns ns, libs []string) []string {
	var n int
	for _, lib := range libs {
		k := key{ns, lib}
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			libs[n] = lib
			n++
		}
	}

	return libs[0:n]
}
