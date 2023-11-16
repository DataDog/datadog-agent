package cache

import (
	"errors"
	"fmt"
	"github.com/cihub/seelog"
	"golang.org/x/exp/slices"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"hash/maphash"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// hashPage is a 4k page of memory.  After two bookkeeping ints,
// it has an array of index entries, followed by the strings themselves.
// The index entries grow from the start of the 4k page to the end,
// and the strings grow from the end of the 4k page to the start.  The
// bookkeeping entries make sure they don't collide.
const hashPageSize = 4096
const numEntries = (hashPageSize - 8) / 8
const callStackDepth = 8

// backBufferSize is padding on each page to handle any memory accesses off
// the end of the last page.  From debugging SIGSEGVs: some string functions are
// implemented with SIMD instructions that read in multiples of 8 bytes.  They
// read past the end of the string and then (presumably) mask off the bytes they
// don't care about.  To avoid seg-faults, keep the last 8 bytes of the buffer
// unused to make sure any reads of short strings don't read off the end of
// allowable address space.  Use 8 bytes in case some loops read the next whole word
// instead of stopping appropriately.
const backBufferSize = 8

// MaxValueSize is the largest possible value we can store.  Start with the page size and take off 16:
// 8 (4+2+2) for a single hashEntry, 8 for the two int32s on the top of hashPage, and 8 for padding.
const MaxValueSize = hashPageSize - 16 - backBufferSize

// MaxProbes is the maximum number of probes going into the hash table.
const MaxProbes = 8

const maxFailedPointers = 2000000
const maxFailedPointersToPrint = 50

// packageSourceDir is the common prefix for source files found during callstack lookup
const packageSourceDir = "/go/src/github.com/DataDog/datadog-agent/pkg"

// containerSummaryPattern will help abbreviate the name of the container.
var containerSummaryPattern = regexp.MustCompile("^container_id://[0-9a-f]{60}([0-9a-f]{4})")

type hashEntry struct {
	hashCode       uint32
	offset, length uint16
}

type hashPage struct {
	indexEntries, stringData int32
	// This array isn't actually this long.  It's 'indexEntries' long
	// and the tail is getting overwritten with strings.  Adding a string grows
	// in two directions simultaneously: the entry is added to the front
	// of the page, and the string itself is prepended to the end.
	// The offset field of each hashEntry object is relative to the
	// address of the hashPage.
	entries [numEntries]hashEntry
}

func (hp *hashPage) insertAtIndex(index, hashcode int, key []byte) bool {
	const entSize = int32(unsafe.Sizeof(hp.entries[0]))
	if hp.stringData < backBufferSize {
		hp.stringData = backBufferSize
	}
	remaining := hashPageSize - (hp.indexEntries*entSize + hp.stringData)
	if remaining < (entSize + int32(len(key))) {
		return false
	}
	copy(hp.entries[index+1:hp.indexEntries+1], hp.entries[index:hp.indexEntries])
	offset := hashPageSize - int(hp.stringData) - len(key)
	stringBuf := unsafe.Slice((*byte)(unsafe.Pointer(hp)), hashPageSize)
	copy(stringBuf[offset:offset+len(key)], key)
	hp.entries[index].hashCode = uint32(hashcode)
	hp.entries[index].length = uint16(len(key))
	hp.entries[index].offset = uint16(offset)
	hp.indexEntries++
	hp.stringData += int32(len(key))
	return true
}

// lookupOrInsert returns the allocated string and true, if it allocated. It
// returns empty string if it didn't fit here, and false. That means we treat
// this as a hash collision and find another page to look into (or insert)
func (hp *hashPage) lookupOrInsert(hcode uint64, key []byte) (string, bool) {
	maskCode := func(hc uint64) int {
		return int(hc & 0xFFFFFFFF)
	}
	maskedHCode := maskCode(hcode)
	index, found := slices.BinarySearchFunc(hp.entries[:hp.indexEntries], hcode,
		func(ent hashEntry, hc uint64) int {
			return int(ent.hashCode) - maskCode(hc)
		})
	if !found {
		if !hp.insertAtIndex(index, maskedHCode, key) {
			return "", false
		}
	}
	return unsafe.String((*byte)(unsafe.Add(unsafe.Pointer(hp), hp.entries[index].offset)),
		int(hp.entries[index].length)), !found
}

type mmapHash struct {
	name           string
	closed         bool
	fd             fs.File
	used, capacity int64 // Bytes used and capacity for strings in the
	seeds          []maphash.Seed
	seedHist       []uint64 // Histograms of lookups that succeeded with the Nth seed.
	pages          []hashPage
	mapping        []byte // This is virtual address space, not memory used.
	closeOnRelease bool
	// value-length statistics, Welford's online variance algorithm
	valueCount uint64
	valueMean  float64
	valueM2    float64
	lock       sync.Mutex
}

type failedPointer struct {
	origin string
	count  int
}

// mmapAllRecord holds every mmapHash created.  This isn't for permanent use,
// just debugging and validation.
type mmapAllRecord struct {
	// When we actually delete, make this nil.
	hashes   []*mmapHash
	origins  map[string]int
	pointers map[uintptr]failedPointer
	lock     sync.Mutex
}

var allMmaps = mmapAllRecord{
	hashes:   make([]*mmapHash, 0, 1),
	origins:  make(map[string]int),
	pointers: make(map[uintptr]failedPointer),
}

func normalizeOrigin(origin string) string {
	result := strings.Builder{}
	for _, c := range origin {
		switch c {
		case '!':
			fallthrough
		case '/':
			fallthrough
		case ':':
			fallthrough
		case ' ':
			result.WriteRune('_')
		default:
			result.WriteRune(c)
		}
	}
	return result.String()
}

func newMmapHash(origin string, fileSize int64, prefixPath string, closeOnRelease bool) (*mmapHash, error) {
	if fileSize < hashPageSize {
		return nil, errors.New("file size too small")
	}
	allMmaps.lock.Lock()
	defer allMmaps.lock.Unlock()

	file, err := os.OpenFile(filepath.Join(prefixPath, fmt.Sprintf("%s-%d-%d.dat", normalizeOrigin(origin), len(allMmaps.hashes), fileSize)),
		os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return nil, err
	}

	// Delete the file so that only our open FD keeps the inode alive.
	defer func(name string) {
		_ = os.Remove(name)
	}(file.Name())

	// Create the file, make a hole in it, mmap the hole.
	if _, err = syscall.Seek(int(file.Fd()), int64(fileSize-1), io.SeekStart); err != nil {
		return nil, err
	}
	// The hole requires a real byte after it to materialize.
	if _, err = file.Write(make([]byte, 1)); err != nil {
		return nil, err
	}

	mappedAddresses, err := syscall.Mmap(int(file.Fd()), 0, int(fileSize), syscall.PROT_WRITE|syscall.PROT_READ,
		syscall.MAP_SHARED|syscall.MAP_FILE)
	if err != nil {
		return nil, err
	}
	seeds := make([]maphash.Seed, 0, MaxProbes)
	for i := 0; i < MaxProbes; i++ {
		seeds = append(seeds, maphash.MakeSeed())
	}

	h := &mmapHash{
		name:           origin,
		closed:         false,
		fd:             file,
		used:           0,
		capacity:       fileSize,
		mapping:        mappedAddresses,
		pages:          unsafe.Slice((*hashPage)(unsafe.Pointer(&mappedAddresses[0])), fileSize/hashPageSize),
		seeds:          seeds,
		seedHist:       make([]uint64, len(seeds)),
		closeOnRelease: closeOnRelease,
	}

	allMmaps.hashes = append(allMmaps.hashes, h)

	return h, nil
}

// lookupOrInsert returns a pre-existing or newly created string with the value of key.  It also
// returns a bool indicating whether implementation is full.  If you get an empty string and a true,
// you would be able to allocate this string on some other instance that isn't full.  If you get an
// empty string and a false, the implementation doesn't support this string.  Go ahead and
// heap-allocate the string, then.
func (table *mmapHash) lookupOrInsert(key []byte) (string, bool) {
	keyLen := len(key)
	if keyLen > MaxValueSize {
		// We don't support long strings, punt.
		return "", false
	}

	if table.closed {
		// We don't return strings after finalization.
		_ = log.Error("Attempted to use mmap hash after release!")

		// This will punt the error upwards, which will then allocate somewhere else.
		return "", false
	}
	for n, seed := range table.seeds {
		hash := maphash.Bytes(seed, key)
		page := &table.pages[hash%uint64(len(table.pages))]
		if result, allocated := page.lookupOrInsert(hash, key); result != "" {
			if allocated {
				// Online mean & variance calculation:
				// https://en.wikipedia.org/wiki/Algorithms_for_calculating_variance#Welford's_online_algorithm
				keyLenF := float64(keyLen)
				table.used += int64(keyLen)
				table.valueCount++
				delta := keyLenF - table.valueMean
				table.valueMean += delta / float64(table.valueCount)
				delta2 := keyLenF - table.valueMean
				table.valueM2 += delta * delta2
			}
			table.seedHist[n]++
			return result, false
		}
	}
	return "", true
}

func (table *mmapHash) sizes() (int64, int64) {
	return table.used, table.capacity
}

// stats returns the mean and variance for the lengths of keys inserted into
// this mmapHash. When these values aren't defined, you get NaN back.
func (table *mmapHash) stats() (float64, float64) {
	if table.valueCount < 1 {
		return math.NaN(), math.NaN()
	} else if table.valueCount < 2 {
		return table.valueMean, math.NaN()
	}

	return table.valueMean, table.valueM2 / float64(table.valueCount)
}

// Name of the mmapHash, printable and slightly sanitized.
func (table *mmapHash) Name() string {
	if len(table.name) == 0 {
		return "<empty>"
	} else if table.name[0] == '!' {
		return table.name
	}
	return containerSummaryPattern.ReplaceAllString(table.name, "container-$1")
}

func (table *mmapHash) finalize() {
	table.lock.Lock()
	defer table.lock.Unlock()
	if table.closed {
		_ = log.Warnf("finalize(%p): Already dead.", table)
		return
	}

	address := unsafe.SliceData(table.mapping)
	var closeOnRelease string
	if table.closeOnRelease {
		closeOnRelease = "YES"
	} else {
		closeOnRelease = "NO"
	}
	log.Debugf(fmt.Sprintf("finalize(%s): Invalidating address %p-%p.  Close-on-release=%s",
		table.Name(), address, unsafe.Add(unsafe.Pointer(address), len(table.mapping)),
		closeOnRelease))
	// Make the segment read-only, worry about actual deletion after we have
	// better debugging around page faults.
	var err error
	if table.closeOnRelease {
		err = syscall.Munmap(table.mapping)
		if err != nil {
			_ = log.Errorf("Failed munmap(): %v", err)
		}
		err = table.fd.Close()
		if err != nil {
			_ = log.Errorf("Failed mapping file close(): %v", err)
		}
		table.fd = nil
	} else {
		// Don't close the mapping, just mark it read-only.  This leaks to disk and address space, but it's
		// still better than using up heap.  It also lets us track down reference leaks to this address
		// space without crashing.
		err = syscall.Mprotect(table.mapping, syscall.PROT_READ)
		if err != nil {
			_ = log.Errorf("Failed mprotect(): %v", err)
		}
		table.fd = nil
	}
}

func (table *mmapHash) finalized() bool {
	return table.closed
}

func (table *mmapHash) accessible() bool {
	return !table.finalized() || !table.closeOnRelease
}

type mapCheck struct {
	index        int
	safe, active bool
}

// invalid indicates that the check found a string that should not
// be used anymore.
func (m mapCheck) invalid() bool {
	return m.index >= 0 && !m.active
}

// isMapped returns (index, active, safe) for the string s.  If the address is mapped,
// index >= 0 (else -1).  And if that mapping is still active, we get active=true.  If
// the address is still mapped in the process address space (active or not), we get safe=true.
// Caller must hold lock to all_maps.lock.  If index < 0, the other two return values are
// irrelevant.
func isMapped(s string) mapCheck {
	// TODO: make isMapped lock-free.
	addr := uintptr(unsafe.Pointer(unsafe.StringData(s)))
	var constP *byte
	for n, t := range allMmaps.hashes {
		t.lock.Lock()
		mapAddr := uintptr(unsafe.Pointer(unsafe.SliceData(t.mapping)))
		if mapAddr <= addr && addr <= (mapAddr+unsafe.Sizeof(constP)*uintptr(len(t.mapping))) {
			// Found it.
			inactive := t.finalized()
			safe := t.accessible()
			if inactive {
				if entry, ok := allMmaps.pointers[mapAddr]; !ok {
					if len(allMmaps.pointers) < maxFailedPointers {
						allMmaps.pointers[mapAddr] = failedPointer{
							origin: t.Name(),
							count:  1,
						}
					}
				} else {
					entry.count++
					allMmaps.pointers[mapAddr] = entry
				}
			}
			t.lock.Unlock()
			return mapCheck{
				index:  n,
				safe:   safe,
				active: !inactive,
			}
		}
		t.lock.Unlock()
	}
	// address isn't part of our memory mapping, so it's safe to return.
	return mapCheck{
		index:  -1,
		safe:   true,
		active: false,
	}
}

// logFailedCheck returns a safe value for 'tag'.  Using the 'safe' value from isMapped,
// logFailedCheck will log a failed call to isMapped and
func logFailedCheck(check mapCheck, callsite, tag string) string {
	location := fmt.Sprintf("<%s>", allMmaps.hashes[check.index].Name())
	for i := 0; i < callStackDepth; i++ {
		// skip over logFailedCheck and the in-package call site, just the ones above.
		_, file, line, _ := runtime.Caller(2 + i)
		location = fmt.Sprintf("%s\t[%s:%d]", location,
			strings.Replace(file, packageSourceDir, "PKG", 1), line)
	}
	if _, found := allMmaps.origins[location]; !found {
		addr := unsafe.StringData(tag)
		if check.safe {
			log.Debugf("mmap_hash.%v: %p: Found tag (%s) from dead region, called from %v", callsite, addr, tag, location)
		} else {
			log.Debugf("mmap_hash.%v: %p: Found tag (INACCESSIBLE) from dead region, called from %v", callsite, addr, location)
		}
	}
	allMmaps.origins[location]++
	if check.safe {
		return tag
	}
	return allMmaps.hashes[check.index].Name()
}

// Check a string to make sure it's still valid.  Save a histogram of failures for tracking
func Check(tag string) bool {
	allMmaps.lock.Lock()
	defer allMmaps.lock.Unlock()

	check := isMapped(tag)
	if check.invalid() {
		logFailedCheck(check, "Check", tag)
		return false
	}
	return check.safe
}

// CheckDefault checks a string and returns it if it's valid, or returns an indicator of where
// it was called for debugging.
func CheckDefault(tag string) string {
	allMmaps.lock.Lock()
	defer allMmaps.lock.Unlock()
	check := isMapped(tag)
	if check.invalid() {
		return logFailedCheck(check, "CheckDefault", tag)
	}
	return tag
}

// Report the active and dead mappings, their lookup depths, and all the failed lookup checks.
func Report() {
	level, err := log.GetLogLevel()
	if err != nil {
		// Weird, log the logging level.
		_ = log.Errorf("Report: GetLogLevel: %v", err)
	} else if level > seelog.DebugLvl {
		// Nothing here will get printed, so don't bother doing the work.
		return
	}
	allMmaps.lock.Lock()
	defer allMmaps.lock.Unlock()
	p := message.NewPrinter(language.English)
	nrHashes := len(allMmaps.hashes)
	type originData struct {
		name                                 string
		totalValues                          uint64
		totalActiveAllocated, totalAllocated int64
	}

	mapData := make(map[string]originData)
	for n, t := range allMmaps.hashes {
		var status string
		name := t.Name()
		data := mapData[name]
		data.name = name
		data.totalValues += t.valueCount
		data.totalAllocated += t.capacity
		if t.finalized() {
			status = "INACTIVE"
		} else {
			data.totalActiveAllocated += t.capacity
			status = "ACTIVE"
		}
		mapData[name] = data

		mean, variance := t.stats()
		log.Debug(p.Sprintf("> %d/%d: %8s Origin=\"%s\" mmap range starting at %p: %v bytes."+
			" Used: %11d, capacity: %11d.  Utilization: %4.1f%%. Mean len: %4.2f, "+
			"Stddev len: %4.2f. Lookup depth: %10d %6d %5d %5d %3d %2d %2d %d", n+1, nrHashes, status,
			t.Name(), unsafe.Pointer(unsafe.SliceData(t.mapping)), len(t.mapping),
			t.used, t.capacity, 100.0*float64(t.used)/float64(t.capacity), mean, math.Sqrt(variance),
			t.seedHist[0], t.seedHist[1], t.seedHist[2], t.seedHist[3],
			t.seedHist[4], t.seedHist[5], t.seedHist[6], t.seedHist[7]))
	}

	for k, v := range mapData {
		log.Debug(p.Sprintf("* %40s: Total Values: %d, Active Bytes Allocated: %d, Total Bytes Allocated: %d",
			k, v.totalValues, v.totalActiveAllocated, v.totalAllocated))
	}

	nrChecks := len(allMmaps.origins)
	count := 1
	totalFailedChecks := 0
	for k, v := range allMmaps.origins {
		log.Debug(p.Sprintf("- %3d/%d %12d/ failed checks: %s", count, nrChecks, v, k))
		totalFailedChecks += v
		count++
	}

	if totalFailedChecks > 0 {
		log.Info(p.Sprintf("Failed Checks Total %d on %d different locations", totalFailedChecks, len(allMmaps.origins)))
	}

	if len(allMmaps.pointers) < maxFailedPointersToPrint {
		for ptr, entry := range allMmaps.pointers {
			log.Debug(p.Sprintf("Address 0x%016x in %s: %d hits", ptr, entry.origin, entry.count))
		}
	}
	log.Debugf(p.Sprintf("Too many (%d) pointers saved.", len(allMmaps.pointers)))
}
