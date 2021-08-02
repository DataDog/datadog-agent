package so

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/common"
)

var regexProcPidMapEntry *regexp.Regexp

const regexProcPidMapEntry_NR = 8

func init() {
	regexProcPidMapEntry = regexp.MustCompile(`(?P<VaddrStart>[\da-f]+)-(?P<VaddrEnd>[\da-f]+) (?P<Permission>[-rwxsp]+) (?P<Offset>[\da-f]+) (?P<Device>[\da-f:]+) (?P<Inode>\d+)[ ]*(?P<Pathname>.*)`)
}

type ProcPidMaps struct {
	ProcPidPath string
	Libraries   map[string]ProcPidMapLibrary
}

type ProcPidMapLibrary struct {
	Mapping []*ProcPidMapEntry
}

type ProcPidMapEntry struct {
	VaddrStart uint64
	VaddrEnd   uint64
	Permission string
	Offset     uint64
	Device     string
	Inode      uint64
	Pathname   string
}

func (l ProcPidMapLibrary) String() string {
	b := ""
	for _, entry := range l.Mapping {
		b += fmt.Sprintf("%+v\n", entry)
	}
	return b
}

func ParseProcPidMaps(pidPath string, buffer *bufio.Reader) (*ProcPidMaps, error) {
	f, err := os.Open(filepath.Join(pidPath, "/maps"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buffer.Reset(f)

	return parseMaps(pidPath, buffer)
}

func parseMaps(pidPath string, buffer *bufio.Reader) (*ProcPidMaps, error) {
	procPidMaps := &ProcPidMaps{
		ProcPidPath: pidPath,
		Libraries:   make(map[string]ProcPidMapLibrary),
	}

	lastPathname := ""
	for {
		line, _, err := buffer.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		entry, err := procPidMapLineToEntry(string(line))
		if err != nil {
			return nil, err
		}
		pathname := entry.Pathname
		if pathname == "" { /* add anonymous entry to the latest library */
			pathname = lastPathname
		}
		lib := procPidMaps.Libraries[pathname]
		lib.Mapping = append(lib.Mapping, entry)
		procPidMaps.Libraries[pathname] = lib
		lastPathname = pathname
	}

	return procPidMaps, nil
}

// GetSharedLibraries() takes an opitonal regex filter in parameter
// and return an array of pathname that match the filter.
//
// Example:
// 7f135146b000-7f135147a000 r--p 00000000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
// 7f135147a000-7f1351521000 r-xp 0000f000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
// 7f1351521000-7f13515b8000 r--p 000b6000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
// 7f13515b8000-7f13515b9000 r--p 0014c000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
//
// Would return ["/usr/lib/x86_64-linux-gnu/libm-2.31.so"]
func (p *ProcPidMaps) GetSharedLibraries(filter ...*regexp.Regexp) []string {
	set := common.NewStringSet()
	for pathname := range p.Libraries {
		if len(filter) == 0 || filter[0].MatchString(pathname) {
			set.Add(pathname)
		}
	}
	return set.GetAll()
}

func procPidMapLineToEntry(line string) (*ProcPidMapEntry, error) {
	stringEntry := regexProcPidMapEntry.FindStringSubmatch(line)
	if len(stringEntry) != regexProcPidMapEntry_NR {
		return nil, fmt.Errorf("parsing map entry error : '%s' '%+v'", line, stringEntry)
	}
	var u uint64
	var err error
	entry := &ProcPidMapEntry{}
	for i, field := range regexProcPidMapEntry.SubexpNames() {
		str := stringEntry[i]
		switch field {
		case "VaddrStart":
			u, err = strconv.ParseUint(str, 16, 64)
			if err != nil {
				return nil, fmt.Errorf("field VaddrStart parse error '%s' : %s", str, err)
			}
			entry.VaddrStart = u
		case "VaddrEnd":
			u, err = strconv.ParseUint(str, 16, 64)
			if err != nil {
				return nil, fmt.Errorf("field VaddrEnd parse error '%s' : %s", str, err)
			}
			entry.VaddrEnd = u
		case "Permission":
			entry.Permission = str
		case "Offset":
			u, err = strconv.ParseUint(str, 16, 64)
			if err != nil {
				return nil, fmt.Errorf("field Offset parse error '%s' : %s", str, err)
			}
			entry.Offset = u
		case "Device":
			entry.Device = str
		case "Inode":
			u, err = strconv.ParseUint(str, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("field Inode parse error '%s' : %s", str, err)
			}
			entry.Offset = u
		case "Pathname":
			entry.Pathname = str
		}
	}
	return entry, nil
}
