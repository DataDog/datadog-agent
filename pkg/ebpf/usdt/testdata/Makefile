# Rebuilds the USDT test fixtures from sdt_fixture.c: run `make` in this
# directory. The fixtures are committed, so that the tests need no compiler.
# Any GCC or Clang cross-compiler with GNU ld works, e.g. for Clang:
#   make CC_X86_64="clang --target=x86_64-linux-gnu" CC_ARM64="clang --target=aarch64-linux-gnu"

CC_X86_64 ?= x86_64-linux-gnu-gcc
CC_ARM64 ?= aarch64-linux-gnu-gcc

# Small, deterministic shared objects: no libc, no unwind tables, no build id,
# no compiler ident, symbol table stripped, 4 KiB pages. The segments are
# linked at 0x10000 so that virtual addresses differ from file offsets.
CFLAGS = -O2 -fPIC -shared -nostdlib -ffreestanding -fno-ident \
	-fno-asynchronous-unwind-tables -fno-unwind-tables
LDFLAGS = -Wl,--build-id=none -Wl,--hash-style=gnu -Wl,-z,max-page-size=0x1000 \
	-Wl,-z,norelro -Wl,-Ttext-segment=0x10000 -Wl,-s

all: sdt_fixture_x86_64.so sdt_fixture_arm64.so

sdt_fixture_x86_64.so: sdt_fixture.c Makefile
	$(CC_X86_64) $(CFLAGS) $(LDFLAGS) -Wl,-z,noseparate-code -o $@ $<

sdt_fixture_arm64.so: sdt_fixture.c Makefile
	$(CC_ARM64) $(CFLAGS) $(LDFLAGS) -o $@ $<

.PHONY: all
