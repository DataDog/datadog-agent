// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"bytes"
	"io"
	"io/fs"
	"math"
	"os"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	garblePatternThreshold = 8
)

// AnalyzeFile examines a file and returns its metadata
func AnalyzeFile(filepath string, fileInfo fs.FileInfo, checkLinkage bool) (*model.FileMetadata, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Get file size
	if fileInfo == nil {
		var err error
		fileInfo, err = file.Stat()
		if err != nil {
			return nil, err
		}
	}

	info := &model.FileMetadata{
		Size:        fileInfo.Size(),
		Compression: int(model.NoCompression),
		Linkage:     int(model.None), // Default to None if linkage check is disabled
	}

	// Check if file is empty
	if fileInfo.Size() == 0 {
		info.Type = int(model.Empty)
		return info, nil
	}

	// Check if file is executable
	info.IsExecutable = isExecutable(fileInfo)

	// Read first few bytes for magic number detection
	header := make([]byte, 20)
	if _, err := file.Read(header); err != nil {
		return nil, err
	}

	// Reset file position
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}

	// Check for compression
	if isComp, compType := isCompressed(header, fileInfo.Size()); isComp {
		info.Compression = int(compType)
		info.Type = int(model.Compressed)
		return info, nil
	}

	if isShellScript(header) {
		info.Type = int(model.ShellScript)
		return info, nil
	}
	if isText(header, fileInfo.Size()) {
		info.Type = int(model.Text)
		return info, nil
	}

	// Determine file type and additional metadata
	err = analyzeBinaryFileType(file, fileInfo, header, info, checkLinkage)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func isExecutable(fileInfo os.FileInfo) bool {
	return fileInfo.Mode()&0111 != 0
}

func isUPX(data []byte) bool {
	// Read first KB of the file
	bytesToCheck := min(len(data), 1024)
	buf := data[:bytesToCheck]

	// Look for "UPX" string
	upx := []byte("UPX")
	pos := bytes.Index(buf, upx)
	if pos == -1 {
		return false
	}

	// Check if the next character is a valid UPX marker
	if pos+3 < len(buf) {
		nextByte := buf[pos+3]
		return nextByte == '!' || nextByte == '0' || nextByte == '1' || nextByte == '2'
	}

	return false
}

func analyzeBinaryFileType(file *os.File, fileInfo os.FileInfo, header []byte, info *model.FileMetadata, checkLinkage bool) error {
	// Read only first 2MB of the file
	maxRead := min(int64(2*1024*1024), fileInfo.Size())
	// Read file data
	data := make([]byte, maxRead)
	n, err := file.Read(data)
	if err != nil && err != io.EOF {
		return err
	}
	data = data[:n]

	// Check for UPX first
	info.IsUPXPacked = isUPX(data)

	garblePatterns := findGarblePatterns(data)
	info.IsGarbleObfuscated = len(garblePatterns) >= garblePatternThreshold

	// Check for common file types
	switch {
	case isELF(header, fileInfo.Size()):
		info.Type = int(model.ELFExecutable)
		err := analyzeELF(file, data, info, checkLinkage)
		if err != nil {
			return err
		}
		return nil

	case isPE(header):
		info.Type = int(model.PEExecutable)
		err := analyzePE(file, info, data, checkLinkage)
		if err != nil {
			return err
		}
		return nil

	case isMachO(header, fileInfo.Size()):
		info.Linkage = int(model.Dynamic)
		info.Type = int(model.MachOExecutable)
		err := analyzeMachO(fileInfo, header, info)
		if err != nil {
			return err
		}
		return nil

	default:
		if isEncrypted(header, fileInfo.Size()) {
			info.Type = int(model.Encrypted)
			return nil
		}
		info.Type = int(model.Binary)
		return nil
	}
}

func isText(header []byte, fileSize int64) bool {
	// Check for shell script shebang
	bytesToCheck := min(int(fileSize), len(header))
	if bytesToCheck >= 2 && header[0] == '#' && header[1] == '!' {
		return true
	}

	// Count non-printable characters
	nonPrintable := 0

	for i := 0; i < bytesToCheck; i++ {
		b := header[i]
		r := rune(b)
		if !unicode.IsPrint(r) && !unicode.IsSpace(r) {
			nonPrintable++
		}
	}

	return float64(nonPrintable)/float64(bytesToCheck) < 0.2
}

func isShellScript(header []byte) bool {
	if len(header) < 2 {
		return false
	}

	// Check for shebang
	if header[0] != '#' || header[1] != '!' {
		return false
	}

	// Convert header to string for shell interpreter check
	headerStr := strings.ToLower(string(header))
	shellInterpreters := []string{
		"#!/bin/sh",
		"#!/bin/bash",
		"#!/bin/dash",
		"#!/bin/ksh",
		"#!/bin/zsh",
		"#!/bin/csh",
		"#!/bin/tcsh",
		"#!/bin/fish",
	}

	for _, interpreter := range shellInterpreters {
		if strings.HasPrefix(headerStr, interpreter) {
			return true
		}
	}
	return false
}

func isCompressed(header []byte, fileSize int64) (bool, model.CompressionType) {
	// Check for common compression signatures
	compressionSignatures := map[model.CompressionType][]byte{
		model.GZip:     {0x1f, 0x8b},                         // gzip
		model.Zip:      {0x50, 0x4b, 0x03, 0x04},             // zip
		model.Zstd:     {0x28, 0xb5, 0x2f, 0xfd},             // zstd
		model.SevenZip: {0x37, 0x7a, 0xbc, 0xaf},             // 7z
		model.BZip2:    {0x42, 0x5a, 0x68},                   // bzip2
		model.XZ:       {0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00}, // xz
	}

	bytesToCheck := min(int(fileSize), len(header))
	for compType, sig := range compressionSignatures {
		if bytesToCheck >= len(sig) && bytes.Equal(header[:len(sig)], sig) {
			return true, compType
		}
	}
	return false, model.NoCompression
}

func isEncrypted(header []byte, fileSize int64) bool {
	// Simple entropy check - high entropy might indicate encryption
	var entropy float64
	counts := make(map[byte]int)

	bytesToCheck := min(int(fileSize), len(header))
	for _, b := range header[:bytesToCheck] {
		counts[b]++
	}

	for _, count := range counts {
		p := float64(count) / float64(bytesToCheck)
		entropy -= p * math.Log2(p)
	}

	return entropy > 7.0 // Threshold for potential encryption
}

// isASCII checks if a byte is a valid ASCII character for function names
func isASCII(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// findGarblePatterns recherche les patterns spécifiques à garble dans les données fournies
func findGarblePatterns(data []byte) map[string]bool {
	patterns := make(map[string]bool)

	// Look for potential function names that match garble pattern
	for i := 0; i < len(data)-4 && i < len(data); i++ {
		// Skip non-ASCII characters
		if !isASCII(data[i]) {
			continue
		}

		// Find the end of the potential function name
		end := i
		for end < len(data) && isASCII(data[end]) {
			end++
		}

		name := string(data[i:end])

		// Skip if name is too short
		if len(name) < 4 {
			continue
		}

		// Split on underscore
		parts := strings.Split(name, "_")
		if len(parts) != 2 {
			continue
		}

		// Check each part
		valid := true
		for _, part := range parts {
			// Each part must be between 2 and 4 characters (garble patterns are usually shorter)
			if len(part) < 2 || len(part) > 4 {
				valid = false
				break
			}

			// Each part must contain at least one letter and one digit
			hasLetter := false
			hasDigit := false
			hasUppercase := false
			for _, c := range part {
				if unicode.IsLetter(c) {
					hasLetter = true
					if unicode.IsUpper(c) {
						hasUppercase = true
					}
				} else if unicode.IsDigit(c) {
					hasDigit = true
				}
			}
			// Garble patterns typically have at least one uppercase letter and one digit
			if !hasLetter || !hasDigit || !hasUppercase {
				valid = false
				break
			}
		}

		if valid {
			patterns[name] = true
		}
	}

	return patterns
}
