// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"crypto/sha256"
	"debug/elf"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <elf-file>\n", os.Args[0])
		os.Exit(1)
	}

	elfPath := os.Args[1]
	if err := processELF(elfPath); err != nil {
		fmt.Fprintf(os.Stderr, "error processing ELF file: %v\n", err)
		os.Exit(1)
	}
}

func processELF(elfPath string) error {
	fmt.Printf("Processing ELF file: %s\n", elfPath)

	f, err := elf.Open(elfPath)
	if err != nil {
		return fmt.Errorf("failed to open ELF file: %w", err)
	}

	ident := make(map[identEntry][]string)

	for _, section := range f.Sections {
		data, err := section.Data()
		if err != nil {
			return fmt.Errorf("failed to read section %s: %w", section.Name, err)
		}

		sha256 := sha256.New()
		if _, err := sha256.Write(data); err != nil {
			return fmt.Errorf("failed to compute SHA256 for section %s: %w", section.Name, err)
		}
		sum := sha256.Sum(nil)
		hash := fmt.Sprintf("%x", sum)

		entry := identEntry{
			size: section.FileSize,
			hash: hash,
		}
		ident[entry] = append(ident[entry], section.Name)
	}

	saved := 0
	for entry, names := range ident {
		if len(names) > 1 {
			fmt.Println(entry.size, names)
			saved += (len(names) - 1) * int(entry.size)
		}
	}
	fmt.Printf("Potential space savings: %d bytes\n", saved)

	return nil
}

type identEntry struct {
	size uint64
	hash string
}
