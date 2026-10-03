"""Magic-number-based file type checks used to filter the install-tree walk."""

import os

_ELF_MAGIC = b"\x7fELF"

# All Mach-O magic numbers we care about: 32/64-bit thin, fat (universal), in
# both byte orders. See <mach-o/loader.h> and <mach-o/fat.h>.
_MACHO_MAGICS = frozenset(
    {
        b"\xfe\xed\xfa\xce",  # MH_MAGIC     (32-bit, big-endian)
        b"\xce\xfa\xed\xfe",  # MH_CIGAM     (32-bit, little-endian)
        b"\xfe\xed\xfa\xcf",  # MH_MAGIC_64  (64-bit, big-endian)
        b"\xcf\xfa\xed\xfe",  # MH_CIGAM_64  (64-bit, little-endian)
        b"\xca\xfe\xba\xbe",  # FAT_MAGIC
        b"\xbe\xba\xfe\xca",  # FAT_CIGAM
        b"\xca\xfe\xba\xbf",  # FAT_MAGIC_64
        b"\xbf\xba\xfe\xca",  # FAT_CIGAM_64
    }
)


def _first_four_bytes(path: str) -> bytes:
    if not os.path.isfile(path):
        return b""
    try:
        with open(path, "rb") as f:
            return f.read(4)
    except OSError:
        return b""


def is_elf(path: str) -> bool:
    """Return True if ``path`` is a regular file whose first four bytes are the ELF magic."""
    return _first_four_bytes(path) == _ELF_MAGIC


def is_macho(path: str) -> bool:
    """Return True if ``path`` is a regular file whose first four bytes are a Mach-O magic.

    Covers thin (32/64-bit) and fat (universal) binaries in either byte order.
    """
    return _first_four_bytes(path) in _MACHO_MAGICS
