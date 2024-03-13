// Software (c) by Dylan Reimerink
//
// This software is licensed under a
// Creative Commons Attribution-ShareAlike 4.0 International License.
//
// You should have received a copy of the license along with this
// work. If not, see <http://creativecommons.org/licenses/by-sa/4.0/>.

package main

import (
	"debug/elf"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/go-delve/delve/pkg/proc"
)

func main() {
	// Use delve to decode the DWARF section
	binInfo := proc.NewBinaryInfo(runtime.GOOS, runtime.GOARCH)
	err := binInfo.AddImage(os.Args[1], 0)
	if err != nil {
		panic(err)
	}

	// Make a list of unique packages
	pkgs := make([]string, 0, len(binInfo.PackageMap))
	for _, fullPkgs := range binInfo.PackageMap {
		for _, fullPkg := range fullPkgs {
			exists := false
			for _, pkg := range pkgs {
				if fullPkg == pkg {
					exists = true
					break
				}
			}
			if !exists {
				pkgs = append(pkgs, fullPkg)
			}
		}
	}
	// Sort them for a nice output
	sort.Strings(pkgs)

	// Parse the ELF file ourselfs
	elfFile, err := elf.Open(os.Args[1])
	if err != nil {
		panic(err)
	}

	// Get the symbol table
	symbols, err := elfFile.Symbols()
	if err != nil {
		panic(err)
	}

	usage := make(map[string]map[string]int)

	for _, sym := range symbols {
		if sym.Section == elf.SHN_UNDEF || sym.Section >= elf.SectionIndex(len(elfFile.Sections)) {
			continue
		}

		sectionName := elfFile.Sections[sym.Section].Name

		symPkg := ""
		for _, pkg := range pkgs {
			if strings.HasPrefix(sym.Name, pkg) {
				symPkg = pkg
				break
			}
		}
		// Symbol doesn't belong to a known package
		if symPkg == "" {
			continue
		}

		pkgStats := usage[symPkg]
		if pkgStats == nil {
			pkgStats = make(map[string]int)
		}

		pkgStats[sectionName] += int(sym.Size)
		usage[symPkg] = pkgStats
	}

	for _, pkg := range pkgs {
		sections, exists := usage[pkg]
		if !exists {
			continue
		}

		fmt.Printf("%s:\n", pkg)
		for section, size := range sections {
			fmt.Printf("%15s: %8d bytes\n", section, size)
		}
		fmt.Println()
	}
}
