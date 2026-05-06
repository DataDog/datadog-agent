# arm64 uprobe single-step repro

Local reproduction steps for the GPU monitoring uprobe warning.

## Prerequisites

- Run on an arm64 host with NVIDIA GPUs.
- `system-probe` must already be running with GPU monitoring enabled.
- The `gpu-burner` repo must be available at `~/dd/gpu-burner`.
- Kernel logs require privileged access (`sudo dmesg -T`).

## Reproduce

From `~/dd/gpu-burner`:

```sh
UV_SKIP_WHEEL_FILENAME_CHECK=1 gdb \
  -ex "set follow-fork-mode child" \
  -ex "run" \
  -ex "quit" \
  --args uv run gpu-burner --run_time 10 manual --matrix_size 1024 --memory_size_gb 0.1
```

The userspace workload may exit normally. The important signal is the kernel
warning.

## Confirm

Check recent kernel logs:

```sh
sudo dmesg -T | python3 -c 'import re,sys; pat=re.compile(r"uprobe_single_step_handler|single_step_handler|do_debug_exception|el0_dbg|WARNING: CPU|Comm: gpu-burner"); [print(line, end="") for line in sys.stdin if pat.search(line)]'
```

Expected signature:

```text
WARNING: CPU: ... PID: ... at arch/arm64/kernel/probes/uprobes.c:190 uprobe_single_step_handler+0x38/0x70
CPU: ... Comm: gpu-burner ...
pc : uprobe_single_step_handler+0x38/0x70
lr : single_step_handler+0x90/0x120
Call trace:
 uprobe_single_step_handler+0x38/0x70
 single_step_handler+0x90/0x120
 do_debug_exception+...
 el0_dbg+...
```

Register values can vary between runs and kernels. The reproduction criterion is
the `uprobe_single_step_handler` warning for the `gpu-burner` process while
system-probe GPU monitoring uprobes are active.
