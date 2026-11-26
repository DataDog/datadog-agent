# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

"""Strip non-entry .BTF.ext line_info records from a stripped ELF64 eBPF object file.

Preserves .BTF, .BTF.ext func_info, and .BTF.ext core_relo intact while
reducing each .BTF.ext line_info section to only the function-entry records
matching func_info offsets (or the first record if a section has no matching
func_info).

Keeping one valid record per function satisfies all three consumers:
- libbpf / bpftool gen min_core_btf (rejects 4-byte empty subsections with EINVAL)
- cilium/ebpf parseLineInfos() (rejects 0-byte subsections with EOF)
- Linux kernel BPF verifier check_btf_line() (requires a line_info entry at each
  function's first instruction whenever line_info_cnt > 0)
"""

from __future__ import annotations

import argparse
import struct
import sys
from dataclasses import dataclass
from pathlib import Path

ELF_MAGIC = b"\x7fELF"
ELFCLASS64 = 2
ELFDATA2LSB = 1
ELFDATA2MSB = 2
ET_REL = 1
SHN_XINDEX = 0xFFFF

SHT_NULL = 0
SHT_RELA = 4
SHT_NOBITS = 8
SHT_REL = 9

ELF64_EHDR_SIZE = 64
ELF64_SHDR_SIZE = 64

BTF_EXT_MAGIC = 0xEB9F
BTF_EXT_BASE_HDR_LEN = 24
BTF_EXT_CORE_HDR_LEN = 32


@dataclass
class _SectionHeader:
    sh_name: int
    sh_type: int
    sh_flags: int
    sh_addr: int
    sh_offset: int
    sh_size: int
    sh_link: int
    sh_info: int
    sh_addralign: int
    sh_entsize: int


def _pad_to(buf: bytearray, align: int) -> None:
    if align <= 1:
        return
    rem = len(buf) % align
    if rem:
        buf.extend(b"\x00" * (align - rem))


def _read_cstr(strtab: bytes, offset: int) -> str:
    if offset < 0 or offset >= len(strtab):
        return ""
    end = strtab.find(b"\x00", offset)
    if end == -1:
        end = len(strtab)
    return strtab[offset:end].decode("utf-8", errors="replace")


def _parse_ext_info_sections(subsection: bytes, endian: str, name: str) -> tuple[int, list[tuple[int, list[bytes]]]]:
    """Parse a .BTF.ext func_info or line_info sub-section into (record_size, [(sec_name_off, [records])])."""
    if len(subsection) < 4:
        raise ValueError(f".BTF.ext {name} too small ({len(subsection)} bytes)")

    (rec_size,) = struct.unpack_from(f"{endian}I", subsection, 0)
    if rec_size < 4 or rec_size > 256 or (rec_size & 3) != 0:
        raise ValueError(f"unexpected .BTF.ext {name} record size: {rec_size}")

    pos = 4
    sections: list[tuple[int, list[bytes]]] = []
    while pos < len(subsection):
        if pos + 8 > len(subsection):
            raise ValueError(f"truncated .BTF.ext {name} section header at offset {pos}")
        sec_name_off, num_info = struct.unpack_from(f"{endian}II", subsection, pos)
        pos += 8
        if num_info == 0:
            raise ValueError(f".BTF.ext {name} section (name_off={sec_name_off}) has zero records")
        sec_bytes = num_info * rec_size
        if pos + sec_bytes > len(subsection):
            raise ValueError(f"truncated .BTF.ext {name} records for section name_off={sec_name_off}")
        records = [subsection[pos + i * rec_size : pos + (i + 1) * rec_size] for i in range(num_info)]
        pos += sec_bytes
        sections.append((sec_name_off, records))

    return rec_size, sections


def strip_btf_ext_line_info(btf_ext: bytes, endian: str = "<") -> bytes:
    """Strip non-entry line_info records from a .BTF.ext payload.

    For each section in line_info, keeps only the first record at each insn_off
    present in that section's func_info (or the first record if no func_info
    matches). This removes >99% of line_info bytes while keeping .BTF.ext valid
    for libbpf/bpftool, cilium/ebpf, and the Linux kernel BPF verifier.
    """
    if len(btf_ext) < BTF_EXT_BASE_HDR_LEN:
        raise ValueError(f".BTF.ext too small ({len(btf_ext)} bytes)")

    (
        magic,
        version,
        flags,
        hdr_len,
        func_info_off,
        func_info_len,
        line_info_off,
        line_info_len,
    ) = struct.unpack_from(f"{endian}HBBIIIII", btf_ext, 0)

    if magic != BTF_EXT_MAGIC:
        raise ValueError(f"invalid .BTF.ext magic: {magic:#x} (expected {BTF_EXT_MAGIC:#x})")
    if version != 1:
        raise ValueError(f"unsupported .BTF.ext version: {version}")
    if flags != 0:
        raise ValueError(f"unsupported .BTF.ext flags: {flags:#x}")
    if hdr_len < BTF_EXT_BASE_HDR_LEN or hdr_len > len(btf_ext):
        raise ValueError(f"invalid .BTF.ext hdr_len: {hdr_len} (section size {len(btf_ext)})")

    has_core_relo = hdr_len >= BTF_EXT_CORE_HDR_LEN
    if has_core_relo:
        core_relo_off, core_relo_len = struct.unpack_from(f"{endian}II", btf_ext, BTF_EXT_BASE_HDR_LEN)
        extra_hdr = btf_ext[BTF_EXT_CORE_HDR_LEN:hdr_len]
    else:
        core_relo_off, core_relo_len = 0, 0
        extra_hdr = btf_ext[BTF_EXT_BASE_HDR_LEN:hdr_len]

    payload = btf_ext[hdr_len:]
    payload_len = len(payload)

    for name, off, length in (
        ("func_info", func_info_off, func_info_len),
        ("line_info", line_info_off, line_info_len),
        ("core_relo", core_relo_off, core_relo_len),
    ):
        if length and off + length > payload_len:
            raise ValueError(f".BTF.ext {name} [{off}:{off + length}] exceeds payload size {payload_len}")

    known_ends = [
        off + length
        for off, length in (
            (func_info_off, func_info_len),
            (line_info_off, line_info_len),
            (core_relo_off, core_relo_len) if has_core_relo else (0, 0),
        )
        if length > 0
    ]
    max_end = max(known_ends, default=0)
    trailing = payload[max_end:]
    if len(trailing) > 3 or any(b != 0 for b in trailing):
        raise ValueError(f".BTF.ext contains {len(trailing)} unexpected trailing bytes beyond known subsections")

    # Empty line_info sub-section: nothing to strip.
    if line_info_len == 0:
        return btf_ext

    func_info = payload[func_info_off : func_info_off + func_info_len]
    line_info = payload[line_info_off : line_info_off + line_info_len]
    core_relo = payload[core_relo_off : core_relo_off + core_relo_len] if (has_core_relo and core_relo_len) else b""

    func_offs_by_sec: dict[int, set[int]] = {}
    if func_info_len > 0:
        _, func_sections = _parse_ext_info_sections(func_info, endian, "func_info")
        for sec_name_off, records in func_sections:
            func_offs_by_sec[sec_name_off] = {struct.unpack_from(f"{endian}I", rec, 0)[0] for rec in records}

    line_rec_size, line_sections = _parse_ext_info_sections(line_info, endian, "line_info")

    new_line_info = bytearray(struct.pack(f"{endian}I", line_rec_size))
    changed = False
    for sec_name_off, records in line_sections:
        target_offs = func_offs_by_sec.get(sec_name_off, set())
        seen_offs: set[int] = set()
        kept: list[bytes] = []
        for rec in records:
            (insn_off,) = struct.unpack_from(f"{endian}I", rec, 0)
            if insn_off in target_offs and insn_off not in seen_offs:
                kept.append(rec)
                seen_offs.add(insn_off)
        if not kept:
            kept.append(records[0])
        if len(kept) != len(records):
            changed = True
        new_line_info.extend(struct.pack(f"{endian}II", sec_name_off, len(kept)))
        for rec in kept:
            new_line_info.extend(rec)

    if not changed:
        return btf_ext

    new_payload = bytearray()

    new_func_info_off = 0
    new_payload.extend(func_info)
    _pad_to(new_payload, 4)

    new_line_info_off = len(new_payload)
    new_line_info_len = len(new_line_info)
    new_payload.extend(new_line_info)
    _pad_to(new_payload, 4)

    if has_core_relo:
        if core_relo_len > 0:
            new_core_relo_off = len(new_payload)
            new_payload.extend(core_relo)
        else:
            new_core_relo_off = 0

    new_header = bytearray(
        struct.pack(
            f"{endian}HBBIIIII",
            magic,
            version,
            flags,
            hdr_len,
            new_func_info_off,
            func_info_len,
            new_line_info_off,
            new_line_info_len,
        )
    )
    if has_core_relo:
        new_header.extend(struct.pack(f"{endian}II", new_core_relo_off, core_relo_len))
    new_header.extend(extra_hdr)

    return bytes(new_header) + bytes(new_payload)


def strip_elf_btf_line_info(elf_bytes: bytes) -> bytes:
    """Rewrite an ELF64 ET_REL object file to strip non-entry .BTF.ext line_info records.

    Returns the input bytes unchanged if there is no .BTF.ext section or if its
    line_info sub-section is already minimal.
    """
    if len(elf_bytes) < ELF64_EHDR_SIZE or elf_bytes[:4] != ELF_MAGIC:
        raise ValueError("input is not an ELF file")

    ei_class = elf_bytes[4]
    ei_data = elf_bytes[5]
    if ei_class != ELFCLASS64:
        raise ValueError(f"unsupported ELF class: {ei_class} (expected ELFCLASS64)")
    if ei_data == ELFDATA2LSB:
        endian = "<"
    elif ei_data == ELFDATA2MSB:
        endian = ">"
    else:
        raise ValueError(f"unsupported ELF endianness: {ei_data}")

    (
        e_type,
        _e_machine,
        _e_version,
        _e_entry,
        _e_phoff,
        e_shoff,
        _e_flags,
        e_ehsize,
        _e_phentsize,
        e_phnum,
        e_shentsize,
        e_shnum,
        e_shstrndx,
    ) = struct.unpack_from(f"{endian}HHIQQQIHHHHHH", elf_bytes, 16)

    if e_type != ET_REL:
        raise ValueError(f"unsupported ELF type: {e_type} (expected ET_REL)")
    if e_phnum != 0:
        raise ValueError(f"unexpected program headers in relocatable object (e_phnum={e_phnum})")
    if e_ehsize < ELF64_EHDR_SIZE:
        raise ValueError(f"invalid ELF header size: {e_ehsize}")
    if e_shentsize != ELF64_SHDR_SIZE:
        raise ValueError(f"unexpected section header entry size: {e_shentsize}")
    if e_shoff == 0:
        return elf_bytes

    shdr_fmt = f"{endian}IIQQQQIIQQ"
    if e_shoff + ELF64_SHDR_SIZE > len(elf_bytes):
        raise ValueError("section header table exceeds file size")

    sh0 = _SectionHeader(*struct.unpack_from(shdr_fmt, elf_bytes, e_shoff))
    shnum = sh0.sh_size if e_shnum == 0 else e_shnum
    shstrndx = sh0.sh_link if e_shstrndx == SHN_XINDEX else e_shstrndx

    if shnum == 0:
        return elf_bytes
    if e_shoff + shnum * ELF64_SHDR_SIZE > len(elf_bytes):
        raise ValueError("section header table extends past EOF")
    if shstrndx >= shnum:
        raise ValueError(f"invalid e_shstrndx: {shstrndx} (shnum={shnum})")

    sections = [
        _SectionHeader(*struct.unpack_from(shdr_fmt, elf_bytes, e_shoff + i * ELF64_SHDR_SIZE)) for i in range(shnum)
    ]

    shstr_sec = sections[shstrndx]
    shstrtab = elf_bytes[shstr_sec.sh_offset : shstr_sec.sh_offset + shstr_sec.sh_size]

    btf_ext_idx: int | None = None
    for idx, sec in enumerate(sections):
        if _read_cstr(shstrtab, sec.sh_name) == ".BTF.ext":
            if btf_ext_idx is not None:
                raise ValueError("multiple .BTF.ext sections found")
            btf_ext_idx = idx

    if btf_ext_idx is None:
        return elf_bytes

    for idx, sec in enumerate(sections):
        if sec.sh_type in (SHT_REL, SHT_RELA) and sec.sh_info == btf_ext_idx:
            rel_name = _read_cstr(shstrtab, sec.sh_name)
            raise ValueError(
                f"relocation section {rel_name!r} (index {idx}) targets .BTF.ext; strip .rel.BTF.ext first"
            )

    btf_ext_sec = sections[btf_ext_idx]
    orig_btf_ext = elf_bytes[btf_ext_sec.sh_offset : btf_ext_sec.sh_offset + btf_ext_sec.sh_size]
    new_btf_ext = strip_btf_ext_line_info(orig_btf_ext, endian=endian)
    if new_btf_ext == orig_btf_ext:
        return elf_bytes

    # Rebuild ELF sections in ascending original file-offset order while preserving
    # every section's index, alignment, flags, links, and contents (except .BTF.ext).
    out = bytearray(elf_bytes[:e_ehsize])
    new_offsets = [0] * shnum
    new_sizes = [sec.sh_size for sec in sections]

    ordered_indices = sorted(
        (idx for idx in range(shnum) if sections[idx].sh_type != SHT_NULL),
        key=lambda idx: (sections[idx].sh_offset, idx),
    )

    for idx in ordered_indices:
        sec = sections[idx]
        align = sec.sh_addralign if sec.sh_addralign > 1 else 1
        _pad_to(out, align)
        new_offsets[idx] = len(out)

        if sec.sh_type == SHT_NOBITS:
            continue
        if idx == btf_ext_idx:
            out.extend(new_btf_ext)
            new_sizes[idx] = len(new_btf_ext)
        else:
            if sec.sh_offset + sec.sh_size > len(elf_bytes):
                raise ValueError(
                    f"section {idx} [{sec.sh_offset}:{sec.sh_offset + sec.sh_size}] exceeds file size {len(elf_bytes)}"
                )
            out.extend(elf_bytes[sec.sh_offset : sec.sh_offset + sec.sh_size])

    _pad_to(out, 8)
    new_shoff = len(out)

    for idx, sec in enumerate(sections):
        out.extend(
            struct.pack(
                shdr_fmt,
                sec.sh_name,
                sec.sh_type,
                sec.sh_flags,
                sec.sh_addr,
                new_offsets[idx],
                new_sizes[idx],
                sec.sh_link,
                sec.sh_info,
                sec.sh_addralign,
                sec.sh_entsize,
            )
        )

    # Patch e_shoff in the ELF64 header (byte offset 40).
    struct.pack_into(f"{endian}Q", out, 40, new_shoff)
    return bytes(out)


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Strip non-entry .BTF.ext line_info records from an eBPF ELF64 object file."
    )
    parser.add_argument("--input", required=True, type=Path, help="Input ELF64 eBPF object file.")
    parser.add_argument("--output", required=True, type=Path, help="Output ELF64 eBPF object file.")
    return parser.parse_args(argv)


def main(argv: list[str]) -> int:
    args = parse_args(argv)
    data = args.input.read_bytes()
    stripped = strip_elf_btf_line_info(data)
    args.output.write_bytes(stripped)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
