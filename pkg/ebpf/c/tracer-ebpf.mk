SHELL=/bin/bash -o pipefail
DEST_DIR?=/ebpf
LINUX_HEADERS=$(shell rpm -q kernel-devel --last | head -n 1 | awk -F'kernel-devel-' '{print "/usr/src/kernels/"$$2}' | cut -d " " -f 1)
FLAGS=-D__KERNEL__ -D__ASM_SYSREG_H -D__BPF_TRACING__ -DCIRCLE_BUILD_URL=\"$(CIRCLE_BUILD_URL)\" \
		-Wno-unused-value \
		-Wno-pointer-sign \
		-Wno-compare-distinct-pointer-types \
		-Wunused \
		-Wall \
		-Werror \
		-O2 -emit-llvm -c /ebpf/c/tracer-ebpf.c \
		$(foreach path,$(LINUX_HEADERS), -I $(path)/arch/x86/include -I $(path)/arch/x86/include/generated -I $(path)/include -I $(path)/include/generated/uapi -I $(path)/arch/x86/include/uapi -I $(path)/include/uapi)

build:
	@sudo mkdir -p "$(DEST_DIR)"
	sudo clang $(FLAGS) -o - | llc -march=bpf -filetype=obj -o "${DEST_DIR}/c/tracer-ebpf.o"
	sudo clang -DDEBUG=1 $(FLAGS) -o - | llc -march=bpf -filetype=obj -o "${DEST_DIR}/c/tracer-ebpf-debug.o"

install:
	go-bindata -pkg ebpf -prefix "${DEST_DIR}/c" -modtime 1 -o "${DEST_DIR}/tracer-ebpf.go" "${DEST_DIR}/c/tracer-ebpf.o" "${DEST_DIR}/c/tracer-ebpf-debug.o"
