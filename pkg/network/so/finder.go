package so

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
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

func (f *finder) Find(filter *regexp.Regexp) (result []Library) {
	mapLib := make(map[libraryKey]Library)
	iteratePIDS(f.procRoot, func(pidPath string, info os.FileInfo, mntNS ns) {
		libs := getSharedLibraries(pidPath, f.buffer, filter)

		for _, lib := range libs {
			k := libraryKey{
				Pathname:       lib,
				MountNameSpace: mntNS,
			}
			if m, ok := mapLib[k]; ok {
				m.PidsPath = append(m.PidsPath, pidPath)
				continue
			}

			/* per PID we add mountInfo and resolv the host path */
			mountInfo := getMountInfo(pidPath, f.buffer)
			/* some /proc/pid/mountinfo could be empty */
			if mountInfo == nil || len(mountInfo.mounts) == 0 {
				continue
			}

			hostPath := f.pathResolver.Resolve(lib, mountInfo)
			if hostPath == "" {
				continue
			}

			mapLib[k] = Library{
				libraryKey: k,
				HostPath:   hostPath,
				PidsPath:   []string{pidPath},
				MountInfo:  mountInfo,
			}
		}
	})
	for _, l := range mapLib {
		result = append(result, l)
	}
	return result
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
