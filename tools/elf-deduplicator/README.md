# ELF Deduplicator

A Rust tool for deduplicating sections in ELF files by sharing common data blocks. Particularly useful for eBPF programs that often contain many sections with identical data but different metadata.

## Usage

```bash
elf-deduplicator --input <INPUT> --output <OUTPUT> [--verbose]
```

### Options

- `-i, --input <INPUT>`: Path to the input ELF file
- `-o, --output <OUTPUT>`: Path for the output deduplicated ELF file  
- `-v, --verbose`: Enable verbose output showing deduplication progress
- `-h, --help`: Show help information

## Example

```bash
# Basic usage
./target/debug/elf-deduplicator -i input.o -o output.o

# With verbose output
./target/debug/elf-deduplicator -i input.o -o output.o --verbose
```

## How it works

1. **Parse ELF**: Uses the `object` and `goblin` crates to parse the input ELF file
2. **Extract sections**: Extracts all section data and metadata
3. **Deduplicate**: Uses SHA-256 hashing to identify sections with identical data
4. **Reconstruct**: Rebuilds the ELF file with shared data blocks, updating section headers to point to the deduplicated data

## Test Results

Testing with Datadog Agent's runtime-security eBPF files:

### runtime-security.o
- **Original size**: 18,820,192 bytes
- **Deduplicated size**: 17,950,408 bytes  
- **Savings**: 869,784 bytes (4.62%)

### runtime-security-fentry.o
- **Original size**: 16,177,904 bytes
- **Deduplicated size**: 15,344,328 bytes
- **Savings**: 833,576 bytes (5.15%)

The tool successfully identifies and deduplicates many sections, particularly those corresponding to similar syscall handlers (e.g., `sys_chmod` vs `compat_sys_chmod`) and their associated relocation sections.

## Dependencies

- `object`: ELF parsing and manipulation
- `goblin`: Additional ELF format support  
- `clap`: Command-line argument parsing
- `anyhow`: Error handling
- `sha2`: SHA-256 hashing for deduplication
- `indexmap`: Ordered collections

## Building

```bash
cargo build --release
```

## Features

- ✅ Deduplicates identical section data
- ✅ Preserves section metadata (names, addresses, alignment, flags)
- ✅ Maintains valid ELF structure
- ✅ Command-line interface with verbose output
- ✅ SHA-256 based content hashing for reliable deduplication
- ✅ Tested with eBPF ELF files

## Limitations

- Currently handles ELF reconstruction in a simplified manner
- Assumes little-endian byte order for section header writing
- Basic error handling - more robust validation could be added