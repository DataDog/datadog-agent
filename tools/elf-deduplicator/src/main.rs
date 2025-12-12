use anyhow::{Context, Result};
use clap::Parser;
use object::{Object, ObjectSection};
use sha2::{Digest, Sha256};
use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;

#[derive(Parser)]
#[command(name = "elf-deduplicator")]
#[command(about = "Deduplicate sections in ELF files by sharing common data blocks")]
struct Args {
    #[arg(short, long)]
    input: PathBuf,
    
    #[arg(short, long)]
    output: PathBuf,
    
    #[arg(short, long, default_value = "false")]
    verbose: bool,
}

#[derive(Clone, Debug)]
struct SectionInfo {
    name: String,
    data: Vec<u8>,
}

#[derive(Clone, Debug)]
struct DataBlock {
    data: Vec<u8>,
    sections: Vec<usize>,
}

fn main() -> Result<()> {
    let args = Args::parse();
    
    let input_data = fs::read(&args.input)
        .with_context(|| format!("Failed to read input file: {:?}", args.input))?;
    
    let elf_file = object::File::parse(&input_data[..])
        .with_context(|| "Failed to parse ELF file")?;
    
    // Only support 64-bit ELF files (including eBPF which is 64-bit)
    if !elf_file.is_64() {
        return Err(anyhow::anyhow!(
            "Only 64-bit ELF files are supported. This appears to be a 32-bit ELF file."
        ));
    }
    
    if args.verbose {
        println!("Processing 64-bit ELF file: {:?}", args.input);
        println!("Architecture: {:?}", elf_file.architecture());
        println!("Endianness: {:?}", elf_file.endianness());
    }
    
    let sections = extract_sections(&elf_file)?;
    let (deduplicated_blocks, section_to_block) = deduplicate_sections(sections, args.verbose)?;
    
    let output_data = reconstruct_elf(&input_data, &deduplicated_blocks, &section_to_block)?;
    
    fs::write(&args.output, output_data)
        .with_context(|| format!("Failed to write output file: {:?}", args.output))?;
    
    if args.verbose {
        println!("Successfully wrote deduplicated ELF to: {:?}", args.output);
    }
    
    Ok(())
}

fn extract_sections(elf_file: &object::File) -> Result<Vec<SectionInfo>> {
    let mut sections = Vec::new();
    
    for section in elf_file.sections() {
        let name = section.name().unwrap_or("<unnamed>").to_string();
        let data = section.data().unwrap_or(&[]).to_vec();
        
        sections.push(SectionInfo {
            name,
            data,
        });
    }
    
    Ok(sections)
}

fn deduplicate_sections(sections: Vec<SectionInfo>, verbose: bool) -> Result<(Vec<DataBlock>, Vec<usize>)> {
    let mut hash_to_block: HashMap<String, usize> = HashMap::new();
    let mut blocks: Vec<DataBlock> = Vec::new();
    let mut section_to_block: Vec<usize> = Vec::new();
    
    for (section_idx, section) in sections.iter().enumerate() {
        let mut hasher = Sha256::new();
        hasher.update(&section.data);
        let hash = format!("{:x}", hasher.finalize());
        
        if let Some(&existing_block_idx) = hash_to_block.get(&hash) {
            blocks[existing_block_idx].sections.push(section_idx);
            section_to_block.push(existing_block_idx);
            
            if verbose {
                println!("Section '{}' shares data with existing block {}", 
                        section.name, existing_block_idx);
            }
        } else {
            let block_idx = blocks.len();
            blocks.push(DataBlock {
                data: section.data.clone(),
                sections: vec![section_idx],
            });
            hash_to_block.insert(hash, block_idx);
            section_to_block.push(block_idx);
            
            if verbose {
                println!("Section '{}' creates new data block {}", 
                        section.name, block_idx);
            }
        }
    }
    
    if verbose {
        let total_original_size: usize = sections.iter().map(|s| s.data.len()).sum();
        let total_deduplicated_size: usize = blocks.iter().map(|b| b.data.len()).sum();
        println!("Original total size: {} bytes", total_original_size);
        println!("Deduplicated size: {} bytes", total_deduplicated_size);
        println!("Space saved: {} bytes ({:.1}%)", 
                total_original_size - total_deduplicated_size,
                100.0 * (total_original_size - total_deduplicated_size) as f64 / total_original_size as f64);
    }
    
    Ok((blocks, section_to_block))
}

fn reconstruct_elf(
    original_data: &[u8],
    blocks: &[DataBlock], 
    section_to_block: &[usize]
) -> Result<Vec<u8>> {
    use goblin::elf::Elf;
    use std::io::{Cursor, Write, Seek, SeekFrom};
    
    let elf = Elf::parse(original_data)?;
    let mut output = Cursor::new(Vec::new());
    
    // Copy ELF header (64-bit only)
    let header_size = 64;
    output.write_all(&original_data[0..header_size])?;
    
    // Calculate new layout
    let mut current_offset = header_size as u64;
    let mut data_blocks_offset = Vec::new();
    
    // Reserve space for section headers (we'll write them at the end)
    let section_headers_offset = current_offset;
    let section_header_size = 64; // 64-bit section headers only
    current_offset += (elf.section_headers.len() * section_header_size) as u64;
    
    // Write data blocks and track their offsets
    for (_block_idx, block) in blocks.iter().enumerate() {
        // Align to 8 bytes
        while current_offset % 8 != 0 {
            current_offset += 1;
        }
        data_blocks_offset.push(current_offset);
        current_offset += block.data.len() as u64;
    }
    
    // Resize output buffer
    output.get_mut().resize(current_offset as usize, 0);
    
    // Write data blocks
    for (block_idx, block) in blocks.iter().enumerate() {
        let offset = data_blocks_offset[block_idx];
        output.seek(SeekFrom::Start(offset))?;
        output.write_all(&block.data)?;
    }
    
    // Update section headers to point to deduplicated data
    output.seek(SeekFrom::Start(section_headers_offset))?;
    for (section_idx, section_header) in elf.section_headers.iter().enumerate() {
        if section_idx < section_to_block.len() {
            let block_idx = section_to_block[section_idx];
            let data_offset = data_blocks_offset[block_idx];
            
            // Create updated section header
            let mut updated_header = section_header.clone();
            updated_header.sh_offset = data_offset;
            
            // Write 64-bit section header
            output.write_all(&updated_header.sh_name.to_le_bytes())?;
            output.write_all(&updated_header.sh_type.to_le_bytes())?;
            output.write_all(&updated_header.sh_flags.to_le_bytes())?;
            output.write_all(&updated_header.sh_addr.to_le_bytes())?;
            output.write_all(&updated_header.sh_offset.to_le_bytes())?;
            output.write_all(&updated_header.sh_size.to_le_bytes())?;
            output.write_all(&updated_header.sh_link.to_le_bytes())?;
            output.write_all(&updated_header.sh_info.to_le_bytes())?;
            output.write_all(&updated_header.sh_addralign.to_le_bytes())?;
            output.write_all(&updated_header.sh_entsize.to_le_bytes())?;
        } else {
            // Copy original section header
            let start = section_headers_offset as usize + section_idx * section_header_size;
            let end = start + section_header_size;
            output.write_all(&original_data[start..end])?;
        }
    }
    
    // Update ELF header to point to our section headers
    let mut result = output.into_inner();
    // Update e_shoff field (offset 40 in 64-bit ELF header)
    let shoff_bytes = section_headers_offset.to_le_bytes();
    result[40..48].copy_from_slice(&shoff_bytes);
    
    Ok(result)
}