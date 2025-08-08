package object

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"testing"
)

func FuzzParseModuleData(f *testing.F) {
	// Create a minimal valid ELF structure for seeding
	validELF := createMinimalELF64()
	f.Add(validELF)

	// Add various malformed ELF structures
	corruptedELF := make([]byte, len(validELF))
	copy(corruptedELF, validELF)
	// Corrupt the section headers
	if len(corruptedELF) > 100 {
		corruptedELF[50] = 0xFF
		corruptedELF[51] = 0xFF
	}
	f.Add(corruptedELF)

	// Add truncated ELF
	if len(validELF) > 10 {
		f.Add(validELF[:len(validELF)/2])
	}

	// Add empty and minimal data
	f.Add([]byte{})
	f.Add([]byte{0x7f, 0x45, 0x4c, 0x46}) // ELF magic only

	f.Fuzz(func(t *testing.T, data []byte) {
		tmpFile, err := os.CreateTemp("", "elf-file")
		if err != nil {
			t.Skip("Failed to create temporary file")
			return
		}
		defer os.Remove(tmpFile.Name())
		if _, err := io.Copy(tmpFile, bytes.NewReader(data)); err != nil {
			t.Skip("Failed to copy ELF file to temporary file")
			return
		}
		mmapFile, err := OpenMMappingElfFile(tmpFile.Name())
		if err != nil {
			// Expected for invalid ELF data
			return
		}
		defer mmapFile.Close()

		// Test ParseModuleData
		moduleData, err := ParseModuleData(mmapFile)
		if err != nil {
			// Error is expected for invalid data
			if len(err.Error()) == 0 {
				t.Errorf("ParseModuleData returned empty error message")
			}
			return
		}

		// If parsing succeeded, test the returned module data
		if moduleData == nil {
			t.Errorf("ParseModuleData returned nil module data without error")
			return
		}

		// Test that fields are reasonable (no obviously corrupted values)
		if moduleData.Text != 0 && moduleData.EText != 0 {
			if moduleData.Text > moduleData.EText {
				t.Errorf("Invalid text range: start %x > end %x", moduleData.Text, moduleData.EText)
			}
		}

		if moduleData.Types != 0 && moduleData.ETypes != 0 {
			if moduleData.Types > moduleData.ETypes {
				t.Errorf("Invalid types range: start %x > end %x", moduleData.Types, moduleData.ETypes)
			}
		}

		// Test GoDebugSections doesn't panic
		debugSections, err := moduleData.GoDebugSections(mmapFile)
		if err == nil && debugSections != nil {
			_ = debugSections.Close()
		}
	})
}

func createMinimalELF64() []byte {
	var buf bytes.Buffer

	// .text section contains a single RET instruction (0xC3)
	text := []byte{0xC3}
	shstrtab := []byte{0x00, '.', 't', 'e', 'x', 't', 0x00, '.', 's', 'h', 's', 't', 'r', 't', 'a', 'b', 0x00}

	// Compute layout
	ehdrSize := uint64(64)
	phdrSize := uint64(56)
	textOffset := ehdrSize + phdrSize
	shstrtabOffset := textOffset + uint64(len(text))
	sectionHeaderOffset := shstrtabOffset + uint64(len(shstrtab))

	// ELF Header
	ident := []byte{
		0x7f, 'E', 'L', 'F',
		2,                   // 64-bit
		1,                   // little endian
		1,                   // version
		0,                   // SYSV
		0, 0, 0, 0, 0, 0, 0, // padding
	}

	ehdr := make([]byte, 64)
	copy(ehdr[0:], ident)
	binary.LittleEndian.PutUint16(ehdr[16:], 2)                   // e_type: ET_EXEC
	binary.LittleEndian.PutUint16(ehdr[18:], 0x3e)                // e_machine: x86_64
	binary.LittleEndian.PutUint32(ehdr[20:], 1)                   // e_version
	binary.LittleEndian.PutUint64(ehdr[24:], 0x400000)            // e_entry
	binary.LittleEndian.PutUint64(ehdr[32:], ehdrSize)            // e_phoff
	binary.LittleEndian.PutUint64(ehdr[40:], sectionHeaderOffset) // e_shoff
	binary.LittleEndian.PutUint16(ehdr[42:], 64)                  // e_ehsize
	binary.LittleEndian.PutUint16(ehdr[44:], 56)                  // e_phentsize
	binary.LittleEndian.PutUint16(ehdr[46:], 1)                   // e_phnum
	binary.LittleEndian.PutUint16(ehdr[48:], 64)                  // e_shentsize
	binary.LittleEndian.PutUint16(ehdr[50:], 3)                   // e_shnum
	binary.LittleEndian.PutUint16(ehdr[52:], 2)                   // e_shstrndx

	// Program Header
	phdr := make([]byte, 56)
	binary.LittleEndian.PutUint32(phdr[0:], 1)         // PT_LOAD
	binary.LittleEndian.PutUint32(phdr[4:], 5)         // PF_R | PF_X
	binary.LittleEndian.PutUint64(phdr[8:], 0)         // p_offset
	binary.LittleEndian.PutUint64(phdr[16:], 0x400000) // p_vaddr
	binary.LittleEndian.PutUint64(phdr[24:], 0x400000) // p_paddr
	binary.LittleEndian.PutUint64(phdr[32:], 0x1000)   // p_filesz
	binary.LittleEndian.PutUint64(phdr[40:], 0x1000)   // p_memsz
	binary.LittleEndian.PutUint64(phdr[48:], 0x1000)   // p_align

	// Section Headers

	// .shstrtab string indexes
	textNameOffset := uint32(1)     // ".text"
	shstrtabNameOffset := uint32(7) // ".shstrtab"

	// Null section header
	shdr0 := make([]byte, 64)

	// .text section header
	shdr1 := make([]byte, 64)
	binary.LittleEndian.PutUint32(shdr1[0:], textNameOffset)
	binary.LittleEndian.PutUint32(shdr1[4:], 1)   // SHT_PROGBITS
	binary.LittleEndian.PutUint64(shdr1[8:], 0x6) // SHF_ALLOC + SHF_EXECINSTR
	binary.LittleEndian.PutUint64(shdr1[16:], 0x400000+textOffset)
	binary.LittleEndian.PutUint64(shdr1[24:], textOffset)
	binary.LittleEndian.PutUint64(shdr1[32:], uint64(len(text)))

	// .shstrtab section header
	shdr2 := make([]byte, 64)
	binary.LittleEndian.PutUint32(shdr2[0:], shstrtabNameOffset)
	binary.LittleEndian.PutUint32(shdr2[4:], 3) // SHT_STRTAB
	binary.LittleEndian.PutUint64(shdr2[24:], shstrtabOffset)
	binary.LittleEndian.PutUint64(shdr2[32:], uint64(len(shstrtab)))

	// Append everything in order
	buf.Write(ehdr)
	buf.Write(phdr)
	buf.Write(text)
	buf.Write(shstrtab)
	buf.Write(shdr0)
	buf.Write(shdr1)
	buf.Write(shdr2)

	return buf.Bytes()
}
