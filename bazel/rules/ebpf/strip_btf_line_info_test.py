# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

import struct
import tempfile
import unittest
from pathlib import Path

from bazel.rules.ebpf.strip_btf_line_info import (
    BTF_EXT_BASE_HDR_LEN,
    BTF_EXT_CORE_HDR_LEN,
    BTF_EXT_MAGIC,
    ELF64_EHDR_SIZE,
    ELF64_SHDR_SIZE,
    ELF_MAGIC,
    ELFCLASS64,
    ELFDATA2LSB,
    ELFDATA2MSB,
    ET_REL,
    SHT_NOBITS,
    SHT_NULL,
    SHT_REL,
    main,
    strip_btf_ext_line_info,
    strip_elf_btf_line_info,
)

SHT_PROGBITS = 1
SHT_SYMTAB = 2
SHT_STRTAB = 3


def _build_btf_ext(
    func_info: bytes,
    line_info: bytes,
    core_relo: bytes | None = None,
    endian: str = "<",
) -> bytes:
    """Build a synthetic .BTF.ext section payload."""
    has_core = core_relo is not None
    hdr_len = BTF_EXT_CORE_HDR_LEN if has_core else BTF_EXT_BASE_HDR_LEN

    payload = bytearray()
    func_off = 0
    payload.extend(func_info)
    while len(payload) % 4:
        payload.append(0)

    line_off = len(payload)
    payload.extend(line_info)
    while len(payload) % 4:
        payload.append(0)

    core_off = len(payload) if (has_core and core_relo) else 0
    if has_core and core_relo:
        payload.extend(core_relo)

    hdr = bytearray(
        struct.pack(
            f"{endian}HBBIIIII",
            BTF_EXT_MAGIC,
            1,
            0,
            hdr_len,
            func_off,
            len(func_info),
            line_off,
            len(line_info),
        )
    )
    if has_core:
        hdr.extend(struct.pack(f"{endian}II", core_off, len(core_relo)))
    return bytes(hdr) + bytes(payload)


def _build_elf64(
    sections: list[tuple[str, int, int, bytes, int, int, int]],
    endian: str = "<",
) -> bytes:
    """Build a minimal valid ELF64 ET_REL object.

    Each entry in `sections` is:
        (name, sh_type, sh_addralign, data, sh_link, sh_info, sh_entsize)
    """
    shstrtab = bytearray(b"\x00")
    name_offsets: list[int] = []
    for name, *_ in sections:
        name_offsets.append(len(shstrtab))
        shstrtab.extend(name.encode("utf-8") + b"\x00")
    shstr_name_off = len(shstrtab)
    shstrtab.extend(b".shstrtab\x00")

    ei_data = ELFDATA2LSB if endian == "<" else ELFDATA2MSB
    e_ident = ELF_MAGIC + bytes([ELFCLASS64, ei_data, 1]) + b"\x00" * 9

    out = bytearray(ELF64_EHDR_SIZE)
    shdrs: list[tuple[int, int, int, int, int, int, int, int, int, int]] = [
        (0, SHT_NULL, 0, 0, 0, 0, 0, 0, 0, 0),
    ]

    for (name, sh_type, align, data, sh_link, sh_info, sh_entsize), name_off in zip(sections, name_offsets, strict=False):
        if align > 1:
            rem = len(out) % align
            if rem:
                out.extend(b"\x00" * (align - rem))
        off = len(out)
        if sh_type != SHT_NOBITS:
            out.extend(data)
        shdrs.append((name_off, sh_type, 0, 0, off, len(data), sh_link, sh_info, align, sh_entsize))

    shstr_off = len(out)
    out.extend(shstrtab)
    shstrndx = len(shdrs)
    shdrs.append((shstr_name_off, SHT_STRTAB, 0, 0, shstr_off, len(shstrtab), 0, 0, 1, 0))

    while len(out) % 8:
        out.append(0)
    shoff = len(out)

    shdr_fmt = f"{endian}IIQQQQIIQQ"
    for sh in shdrs:
        out.extend(struct.pack(shdr_fmt, *sh))

    ehdr = e_ident + struct.pack(
        f"{endian}HHIQQQIHHHHHH",
        ET_REL,
        247,  # EM_BPF
        1,
        0,
        0,
        shoff,
        0,
        ELF64_EHDR_SIZE,
        0,
        0,
        ELF64_SHDR_SIZE,
        len(shdrs),
        shstrndx,
    )
    out[:ELF64_EHDR_SIZE] = ehdr
    return bytes(out)


def _extract_sections(elf_bytes: bytes, endian: str = "<") -> dict[str, tuple[int, int, bytes]]:
    """Return {name: (sh_offset, sh_addralign, content)} for all named sections."""
    (_, _, _, _, _, e_shoff, _, _, _, _, _, e_shnum, e_shstrndx) = struct.unpack_from(
        f"{endian}HHIQQQIHHHHHH", elf_bytes, 16
    )
    shdr_fmt = f"{endian}IIQQQQIIQQ"
    raw = [struct.unpack_from(shdr_fmt, elf_bytes, e_shoff + i * ELF64_SHDR_SIZE) for i in range(e_shnum)]
    shstr_off = raw[e_shstrndx][4]
    shstr_size = raw[e_shstrndx][5]
    shstrtab = elf_bytes[shstr_off : shstr_off + shstr_size]

    result: dict[str, tuple[int, int, bytes]] = {}
    for sh_name, sh_type, _, _, sh_offset, sh_size, _, _, sh_addralign, _ in raw[1:]:
        end = shstrtab.find(b"\x00", sh_name)
        name = shstrtab[sh_name:end].decode("utf-8")
        content = b"" if sh_type == SHT_NOBITS else elf_bytes[sh_offset : sh_offset + sh_size]
        result[name] = (sh_offset, sh_addralign, content)
    return result


class StripBtfLineInfoTest(unittest.TestCase):
    def test_strip_btf_ext_preserves_func_info_and_core_relo(self) -> None:
        func_info = struct.pack("<IIII", 8, 10, 1, 0) + struct.pack("<II", 0, 42)
        line_info = struct.pack("<I", 16) + b"\xaa" * 64
        core_relo = struct.pack("<I", 16) + b"\xbb" * 32

        btf_ext = _build_btf_ext(func_info, line_info, core_relo)
        stripped = strip_btf_ext_line_info(btf_ext)

        self.assertEqual(len(stripped), len(btf_ext) - 64)

        magic, version, flags, hdr_len, f_off, f_len, l_off, l_len, c_off, c_len = struct.unpack_from(
            "<HBBIIIIIII", stripped, 0
        )
        self.assertEqual(magic, BTF_EXT_MAGIC)
        self.assertEqual(version, 1)
        self.assertEqual(flags, 0)
        self.assertEqual(hdr_len, BTF_EXT_CORE_HDR_LEN)
        self.assertEqual(f_off, 0)
        self.assertEqual(f_len, len(func_info))
        self.assertEqual(l_off, len(func_info))
        self.assertEqual(l_len, 4)
        self.assertEqual(c_off, len(func_info) + 4)
        self.assertEqual(c_len, len(core_relo))

        payload = stripped[hdr_len:]
        self.assertEqual(payload[f_off : f_off + f_len], func_info)
        self.assertEqual(struct.unpack_from("<I", payload, l_off)[0], 16)
        self.assertEqual(payload[c_off : c_off + c_len], core_relo)

        # Stripping again must be a no-op (idempotent).
        self.assertEqual(strip_btf_ext_line_info(stripped), stripped)

    def test_strip_btf_ext_24_byte_header_no_core_relo(self) -> None:
        func_info = struct.pack("<IIII", 8, 10, 1, 0)
        line_info = struct.pack("<I", 16) + b"\x11" * 48

        btf_ext = _build_btf_ext(func_info, line_info, core_relo=None)
        stripped = strip_btf_ext_line_info(btf_ext)

        magic, version, flags, hdr_len, f_off, f_len, l_off, l_len = struct.unpack_from("<HBBIIIII", stripped, 0)
        self.assertEqual((magic, version, flags, hdr_len), (BTF_EXT_MAGIC, 1, 0, BTF_EXT_BASE_HDR_LEN))
        self.assertEqual(l_len, 4)
        self.assertEqual(stripped[hdr_len + f_off : hdr_len + f_off + f_len], func_info)
        self.assertEqual(struct.unpack_from("<I", stripped, hdr_len + l_off)[0], 16)

    def test_strip_btf_ext_rejects_unexpected_trailing_bytes(self) -> None:
        func_info = struct.pack("<II", 8, 0)
        line_info = struct.pack("<I", 16) + b"\x22" * 16
        btf_ext = _build_btf_ext(func_info, line_info, core_relo=b"") + b"\x01\x02\x03\x04"

        with self.assertRaisesRegex(ValueError, "unexpected trailing bytes"):
            strip_btf_ext_line_info(btf_ext)

    def test_strip_elf_rebuilds_aligned_sections_and_preserves_other_sections(self) -> None:
        func_info = struct.pack("<I", 8) + b"\x10" * 16
        # Shrink by 12 bytes (not a multiple of 8) to verify subsequent 8-byte aligned sections stay aligned.
        line_info = struct.pack("<I", 16) + b"\x20" * 12
        core_relo = struct.pack("<I", 16) + b"\x30" * 16
        btf_ext = _build_btf_ext(func_info, line_info, core_relo)

        prog_bytes = b"\x95\x00\x00\x00\x00\x00\x00\x00" * 4
        btf_bytes = b"\x9f\xeb" + b"\x00" * 30
        symtab_bytes = b"\x00" * 48

        elf = _build_elf64(
            [
                (".text", SHT_PROGBITS, 8, prog_bytes, 0, 0, 0),
                (".bss", SHT_NOBITS, 8, b"\x00" * 16, 0, 0, 0),
                (".BTF", SHT_PROGBITS, 1, btf_bytes, 0, 0, 0),
                (".BTF.ext", SHT_PROGBITS, 1, btf_ext, 0, 0, 0),
                (".symtab", SHT_SYMTAB, 8, symtab_bytes, 6, 1, 24),
            ]
        )

        stripped_elf = strip_elf_btf_line_info(elf)
        self.assertLess(len(stripped_elf), len(elf))

        sections = _extract_sections(stripped_elf)
        self.assertEqual(sections[".text"][2], prog_bytes)
        self.assertEqual(sections[".BTF"][2], btf_bytes)
        self.assertEqual(sections[".symtab"][2], symtab_bytes)
        self.assertEqual(sections[".symtab"][0] % 8, 0, ".symtab must remain 8-byte aligned")
        self.assertEqual(sections[".BTF.ext"][2], strip_btf_ext_line_info(btf_ext))

        # Section header table must also be 8-byte aligned.
        e_shoff = struct.unpack_from("<Q", stripped_elf, 40)[0]
        self.assertEqual(e_shoff % 8, 0)

    def test_strip_elf_noop_without_btf_ext(self) -> None:
        elf = _build_elf64([(".text", SHT_PROGBITS, 8, b"\x95" * 8, 0, 0, 0)])
        self.assertEqual(strip_elf_btf_line_info(elf), elf)

    def test_strip_elf_rejects_rel_btf_ext(self) -> None:
        btf_ext = _build_btf_ext(struct.pack("<I", 8), struct.pack("<I", 16) + b"\xaa" * 16, b"")
        # Section 1 is .BTF.ext; section 2 is .rel.BTF.ext with sh_info = 1.
        elf = _build_elf64(
            [
                (".BTF.ext", SHT_PROGBITS, 1, btf_ext, 0, 0, 0),
                (".rel.BTF.ext", SHT_REL, 8, b"\x00" * 16, 0, 1, 16),
            ]
        )
        with self.assertRaisesRegex(ValueError, "strip .rel.BTF.ext first"):
            strip_elf_btf_line_info(elf)

    def test_strip_elf_big_endian(self) -> None:
        func_info = struct.pack(">I", 8) + b"\x11" * 8
        line_info = struct.pack(">I", 16) + b"\x22" * 32
        btf_ext = _build_btf_ext(func_info, line_info, core_relo=b"", endian=">")
        elf = _build_elf64([(".BTF.ext", SHT_PROGBITS, 1, btf_ext, 0, 0, 0)], endian=">")

        stripped = strip_elf_btf_line_info(elf)
        sections = _extract_sections(stripped, endian=">")
        self.assertEqual(sections[".BTF.ext"][2], strip_btf_ext_line_info(btf_ext, endian=">"))

    def test_cli_roundtrip(self) -> None:
        btf_ext = _build_btf_ext(struct.pack("<I", 8), struct.pack("<I", 16) + b"\x44" * 32, b"")
        elf = _build_elf64([(".BTF.ext", SHT_PROGBITS, 1, btf_ext, 0, 0, 0)])

        with tempfile.TemporaryDirectory() as directory:
            inp = Path(directory) / "in.o"
            out = Path(directory) / "out.o"
            inp.write_bytes(elf)

            self.assertEqual(main(["--input", str(inp), "--output", str(out)]), 0)
            self.assertEqual(out.read_bytes(), strip_elf_btf_line_info(elf))


if __name__ == "__main__":
    unittest.main()
