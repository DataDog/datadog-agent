//go:build windows

package main

import (
	"fmt"
	"log"
	"syscall"
	"unsafe"
)

// Must mirror MSStoreEntry from the DLL exactly.
// char* → *byte; uint8_t → uint8.
// Add padding so the struct remains 8-byte aligned like MSVC.
type cStoreEntry struct {
	displayName *byte
	version     *byte
	installDate *byte
	source      *byte
	is64bit     uint8
	_           [7]byte // padding to keep 8-byte alignment
	publisher   *byte
	status      *byte
	productCode *byte
}

// Public struct you provided.
type Entry struct {
	DisplayName string `json:"name"`
	Version     string `json:"version"`
	InstallDate string `json:"deployment_time,omitempty"`
	Source      string `json:"software_type"`
	UserSID     string `json:"user,omitempty"`
	Is64Bit     bool   `json:"is_64_bit"`
	Publisher   string `json:"publisher"`
	Status      string `json:"deployment_status"`
	ProductCode string `json:"product_code"`
}

var (
	mod           = syscall.NewLazyDLL("MSStoreApps.dll")
	procList      = mod.NewProc("ListStoreEntries")
	procFree      = mod.NewProc("FreeStoreEntries")
)

func cstr(p *byte) string {
	if p == nil {
		return ""
	}
	base := uintptr(unsafe.Pointer(p))
	var n int
	for *(*byte)(unsafe.Pointer(base + uintptr(n))) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(unsafe.Pointer(p)), n))
}

// List returns MS Store installations mapped to Entry.
func List() ([]Entry, error) {
	var arrPtr uintptr
	var count int32

	r1, _, _ := procList.Call(
		uintptr(unsafe.Pointer(&arrPtr)),
		uintptr(unsafe.Pointer(&count)),
	)
	if r1 != 0 {
		return nil, fmt.Errorf("ListStoreEntries failed: %d", r1)
	}
	if arrPtr == 0 || count == 0 {
		return []Entry{}, nil
	}
	defer procFree.Call(arrPtr, uintptr(count))

	raw := unsafe.Slice((*cStoreEntry)(unsafe.Pointer(arrPtr)), int(count))
	out := make([]Entry, 0, len(raw))
	for _, c := range raw {
		out = append(out, Entry{
			DisplayName: cstr(c.displayName),
			Version:     cstr(c.version),
			InstallDate: cstr(c.installDate),
			Source:      "msstore",
			UserSID:     "",
			Is64Bit:     c.is64bit == 1,
			Publisher:   cstr(c.publisher),
			Status:      "installed",
			ProductCode: cstr(c.productCode),
		})
	}
	return out, nil
}

func main() {
	entries, err := List()
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range entries {
		fmt.Printf("%s\n", e.DisplayName)
		fmt.Printf("  Version: %s\n", e.Version)
		fmt.Printf("  Install Date: %s\n", e.InstallDate)
		fmt.Printf("  Source: %s\n", e.Source)
		fmt.Printf("  User SID: %s\n", e.UserSID)
		fmt.Printf("  Is 64-bit: %v\n", e.Is64Bit)
		fmt.Printf("  Publisher: %s\n", e.Publisher)
		fmt.Printf("  Status: %s\n", e.Status)
		fmt.Printf("  Product Code: %s\n", e.ProductCode)
		fmt.Println()
	}
	fmt.Printf("Total entries: %d\n", len(entries))
}