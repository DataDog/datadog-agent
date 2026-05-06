# Review: `tools/ebpf/trace-arm64-singlestep.bt`

Findings only. Nothing has been changed in the script.

Kernel source referenced: `~/dd/kernel-analysis/linux-aws-6.14-source/src/linux-aws-6.14-6.14.0`.

## 1. Missed `TIF_SINGLESTEP` trigger sites

`user_enable_single_step()` is the canonical setter
(`arch/arm64/kernel/debug-monitors.c:449`, via
`test_and_set_ti_thread_flag(ti, TIF_SINGLESTEP)`). It is `NOKPROBE_SYMBOL`,
so it is unprobeable. Its callers on this kernel:

| Caller | Covered by script? | Notes |
|---|---|---|
| `arch_uprobe_pre_xol` (`arch/arm64/kernel/probes/uprobes.c:80`) | yes | direct kprobe |
| `ptrace_resume` (`kernel/ptrace.c:848`) | yes (kprobe + tracepoint) | `static`; risk of inlining (see below) |
| `breakpoint_handler` (`arch/arm64/kernel/hw_breakpoint.c:678`) | **no** | `NOKPROBE_SYMBOL` |
| `watchpoint_handler` (`arch/arm64/kernel/hw_breakpoint.c:829`) | **no** | `NOKPROBE_SYMBOL` |

Implications:

- The fire of an HBP/WP that flips `TIF_SINGLESTEP` is not observable from
  kprobes on this kernel: the entire upstream chain
  (`do_debug_exception` → `breakpoint_handler` / `watchpoint_handler` →
  `single_step_handler` → `call_step_hook` → `user_enable_single_step`) is
  `NOKPROBE_SYMBOL`. Same blacklist applies to fprobe/kfunc on 6.14.
- What *is* observable is **who configured** the HBP/WP. See section 2a.
  For this investigation that's the answer the script actually needs:
  `gpu-burner` runs under gdb in the repro, gdb sets HBPs/WPs via
  `ptrace(PTRACE_SETREGSET, NT_ARM_HW_BREAK|NT_ARM_HW_WATCH, ...)`, and
  that path is fully kprobeable.
- The header's framing of `perf_event_open` as "the user-facing setup path"
  is also incomplete — it only catches HBP/WP setup via
  `perf_event_open(PERF_TYPE_BREAKPOINT)`. It does **not** catch the ptrace
  regset path that gdb uses by default on arm64.
- `ptrace_resume` is `static` in `kernel/ptrace.c` and only called from
  `ptrace_request` in the same file. Compiler may inline it. If inlined on
  this kernel build, `kprobe:ptrace_resume` will silently fail to attach.
  Verify with `grep ptrace_resume /proc/kallsyms` before relying on it. The
  `tracepoint:syscalls:sys_enter_ptrace` probe is the reliable fallback for
  user-driven cases.
- `PTRACE_SINGLEBLOCK` is not in the predicate, but on arm64
  `arch_has_block_step()` is false, so `user_enable_block_step` is
  unreachable. Predicate is fine as-is.
- `ptrace_disable` (called on `PTRACE_DETACH`) only clears, doesn't set.
- No other in-tree setter on arm64. Bit 21 is `TIF_SINGLESTEP` and no path
  writes it directly outside `user_enable_single_step`.

## 2a. Tracking who configures HBP/WP

If the question is "which process configured the HW breakpoint/watchpoint
whose fire later set `TIF_SINGLESTEP`", probe the registration entry
points instead of the fault handlers. Both are global, kprobeable, not
`NOKPROBE_SYMBOL`:

- `register_user_hw_breakpoint` (`kernel/events/hw_breakpoint.c:741`,
  `EXPORT_SYMBOL_GPL`) — used by the arm64 ptrace regset path
  (`ptrace_hbp_create` → `register_user_hw_breakpoint`). This is the
  `ptrace(PTRACE_SETREGSET, NT_ARM_HW_BREAK|NT_ARM_HW_WATCH, ...)` route
  that gdb uses by default on arm64.
- `register_perf_hw_breakpoint` (`kernel/events/hw_breakpoint.c:713`) —
  used by the `perf_event_open(PERF_TYPE_BREAKPOINT)` path
  (`kernel/events/hw_breakpoint.c:958`).

Together they cover the two practical user-space configuration mechanisms
on arm64.

What you can read at probe time without dereferencing `task_struct`:

| Probe | arg | meaning |
|---|---|---|
| `kprobe:register_user_hw_breakpoint` | `arg0` | `struct perf_event_attr *attr` (target addr/len/type live here) |
| | `arg1` | `perf_overflow_handler_t triggered` (`ptrace_hbptriggered` for the ptrace path) |
| | `arg2` | `void *context` |
| | `arg3` | `struct task_struct *tsk` — the **target** thread |
| | `comm` / `tid` / `pid` | the **actor** (e.g. gdb) |
| `kprobe:register_perf_hw_breakpoint` | `arg0` | `struct perf_event *bp` (its `->attr` has bp_addr/bp_type/bp_len, `->ctx->task` is target if any) |
| | `comm` / `tid` / `pid` | the actor (whatever called `perf_event_open`) |

`register_user_hw_breakpoint` alone gives "actor X configured an HBP/WP on
target task Y" with `tsk` already a raw `task_struct *` arg — no
`task_struct` deref needed. You can match that pointer against the
`child_task=` value the existing `ptrace_resume` kprobe prints, or against
the raw target pointer in any later event.

Optional extras:

- `kprobe:hw_break_set` (`arch/arm64/kernel/ptrace.c:506`) — arm64 ptrace
  regset `->set` callback. `static` but only invoked via the regset
  function pointer, so it must be emitted as a real symbol (same argument
  as `uprobe_single_step_handler`). `arg2` is `note_type`
  (`NT_ARM_HW_BREAK` vs `NT_ARM_HW_WATCH`); the `kiov` carries the full
  request.
- `kprobe:modify_user_hw_breakpoint` — exported, fires when an existing
  HBP/WP slot's address/control is changed after creation. Probing only
  `register_*` misses these updates.
- Drop or demote `kprobe:__arm64_sys_perf_event_open`. It can't tell you
  `PERF_TYPE_BREAKPOINT` without dereferencing user memory, and
  `register_perf_hw_breakpoint` already gives you that signal cleanly.

## 2. Other paths that trigger the uprobe / WARN

The WARN you reproduce is in `uprobe_single_step_handler`:

```text
arch/arm64/kernel/probes/uprobes.c:190
  WARN_ON(utask && (instruction_pointer(regs) != utask->xol_vaddr + 4));
```

The script does **not** probe this function. It is `static` but registered
as `uprobes_step_hook.fn`, so it cannot be inlined and will be in
`/proc/kallsyms`. Adding `kprobe:uprobe_single_step_handler` would let us
log the WARN site directly with `regs` in hand.

Other gaps in uprobe instrumentation:

- `uprobe_breakpoint_handler` — arm64 break-hook entry registered as
  `uprobes_break_hook.fn`. Currently observed indirectly via
  `uprobe_pre_sstep_notifier`. Direct probing would give the entry/exit
  pair plus `esr`.
- `uprobe_post_sstep_notifier` — symmetric counterpart to
  `uprobe_pre_sstep_notifier`, runs immediately after the WARN. Not probed.
- `uprobe_handle_trampoline` — uretprobe trampoline hits in `handle_swbp`
  short-circuit before `find_active_uprobe_rcu` / `handler_chain`:

  ```text
  if (bp_vaddr == uprobe_get_trampoline_vaddr())
      return uprobe_handle_trampoline(regs);
  ```

  Trampoline hits will not appear in `@find_bp_vaddr` or `@last_uprobe_*`.
  This is a real blind spot if system-probe GPU monitoring uses uretprobes
  (it does for some symbols).
- `pre_ssout` — function that decides "we're going to XOL" and calls
  `arch_uprobe_pre_xol`. Probing here with `bp_vaddr` would let us correlate
  `bp_vaddr → xol_vaddr` directly, which is the equality the WARN compares.
- `uprobe_deny_signal` — relevant if a signal lands between pre_xol and the
  single-step trap (one natural way `ip != xol_vaddr+4` could become true).
- `arch_uprobe_xol_was_trapped` — relevant when
  `fault_code != UPROBE_INV_FAULT_CODE`.

## 3. Correlation correctness

### `@last_uprobe_path` mapping

- `arch_uprobe_skip_sstep_ret` sets `path = retval ? 2 : 3`.
  - `retval == 1`: XOL skipped, no single-step set up by uprobe — `path`
    stays `2`.
  - `retval == 0`: kernel falls through to `pre_ssout` →
    `arch_uprobe_pre_xol`, which overwrites to `4`.
- `path == 3` should be transient. If you see it at WARN time, it means
  `pre_ssout` failed before `arch_uprobe_pre_xol` ran — strong evidence the
  single-step seen at WARN is not from this uprobe.
- `path == 4` set in `arch_uprobe_pre_xol`'s kprobe, never advanced when
  `arch_uprobe_post_xol` runs. So `path == 4` is sticky for the entire XOL
  window plus indefinitely after. Don't read it as "currently executing
  XOL".

### `bug_handler` correlation

- `bug_handler` triggers for **every** `WARN`/`BUG` in the kernel, not just
  `uprobe_single_step_handler`'s. The script does no filtering and
  dereferences `@last_uprobe_*` unconditionally.
- When a map entry doesn't exist, bpftrace returns 0, so
  `nsecs - @last_uprobe_ns[tid]` becomes a meaningless huge number. Output
  will be confusing for unrelated WARNs.
- Suggested mitigations: gate the `@last_uprobe_*` print on
  `@last_uprobe_ns[tid] != 0`, or filter `bug_handler` by `regs->pc`
  matching the address of `uprobe_single_step_handler` (resolved once from
  kallsyms).
- `bug_handler` arg labels: `arg0=0x%lx arg1=0x%lx` are actually `regs` and
  `esr`. Mislabeled.
- `bug_handler` runs from a debug exception path. `ustack` unwinding from
  there is generally OK on arm64 but unreliable for frame-pointer-less
  Python/uv binaries.
- `comm` for `bug_handler` is `current` — the target thread for the uprobe
  WARN. The label `actor=` is correct only for the ptrace tracer kprobes;
  for `bug_handler` it actually means "target". Minor.

### `@find_bp_vaddr` lifecycle

- `kretprobe:find_active_uprobe_rcu /retval/` does not delete
  `@find_bp_vaddr[tid]`. Cleanup happens in `kprobe:handler_chain`.
- If `handle_swbp` exits early between `find_active_uprobe_rcu` returning
  non-NULL and `handler_chain` (`!test_bit(UPROBE_COPY_INSN)`,
  `!get_utask()`, or `arch_uprobe_ignore`), the value persists until
  overwritten by the next `find_active_uprobe_rcu` on the same TID.
- For the trampoline path, `find_active_uprobe_rcu` is not called, so
  `@find_bp_vaddr[tid]` echoes the previous BP, not the current trampoline
  hit.

### Ptrace target tid

- `kprobe:ptrace_resume` logs `child_task=0x...` (raw task pointer). Without
  task_struct deref (which crashes bpftrace per the header) you cannot map
  this to a tid in-script. Correlation with the target thread's later
  `arch_uprobe_pre_xol` / WARN must be done out-of-band by inspecting the
  gdb-spawned tid.

### Map growth

- `@last_uprobe_*` maps are never deleted. They grow with the number of
  distinct TIDs that ever hit a uprobe over the trace's lifetime.
  Correctness-OK; bounded by trace duration.

### kretprobe maxactive

- `kretprobe`s on `find_active_uprobe_rcu`, `handle_swbp`,
  `arch_uprobe_*`: if maxactive (default ~`NR_CPUS`) is exceeded under
  load, kretprobes silently drop. Heavy uprobe traffic can mask data.

### Misc

- `kretprobe:find_active_uprobe_rcu` has two probes with predicates
  `/retval/` and `/!retval/`. Both attach to the same retprobe. Uses 2
  maxactive slots per call and 2 program invocations. Not a bug.
- `kprobe:__arm64_sys_perf_event_open` is largely redundant with
  `tracepoint:syscalls:sys_enter_perf_event_open` except for the kstack.
  The syscall wrapper takes `pt_regs *` as `arg0`; any future arg printing
  here must deref `regs->regs[0..]`.

## 4. Style / documentation nits

- Header comment "ptrace_resume below user_enable_single_step()" is
  confusing; `user_enable_single_step` is the *callee* of `ptrace_resume`,
  not above/below.
- `BEGIN` advertises "comm tid pid args/stacks" but several events print
  fields out of that order.
- `#define PTRACE_SINGLESTEP / PTRACE_SYSEMU_SINGLESTEP` is used in exactly
  one filter. Could be inlined.
- No `END` block. On Ctrl-C bpftrace dumps all non-deleted maps
  automatically — useful, but worth calling out so readers don't think it's
  noise.

## 5. Most impactful additions

If you add only a few probes, in priority order:

1. `kprobe:register_user_hw_breakpoint` and
   `kprobe:register_perf_hw_breakpoint`. Identify the process that
   configured each HBP/WP, with the target `task_struct *` in hand. This
   is the only practical way to attribute HBP/WP-induced `TIF_SINGLESTEP`
   on this kernel.
2. `kprobe:uprobe_single_step_handler` (+ kretprobe). Direct WARN site.
   Logs `regs` at the comparison point.
3. `kprobe:uprobe_post_sstep_notifier` (+ kret). Symmetric to the pre
   variant; confirms whether the single-step trap was actually accepted as
   a uprobe one.
4. `kprobe:uprobe_handle_trampoline`. Closes the uretprobe blind spot.
5. Filter `bug_handler` on `regs->pc` matching the address of
   `uprobe_single_step_handler`, or at least gate `@last_uprobe_*` print on
   `@last_uprobe_ns[tid] != 0`.
6. `kprobe:pre_ssout`. Logs `(uprobe, bp_vaddr)` as a clean "intends to
   XOL" record before `arch_uprobe_pre_xol`.

## 6. Sanity-check before running

- Confirm the relevant symbols exist in `/proc/kallsyms`:

  ```sh
  grep -wE 'ptrace_resume|uprobe_single_step_handler|uprobe_breakpoint_handler|uprobe_post_sstep_notifier|uprobe_handle_trampoline|pre_ssout|find_active_uprobe_rcu|handler_chain|arch_uprobe_pre_xol|arch_uprobe_post_xol|arch_uprobe_skip_sstep|arch_uprobe_abort_xol|bug_handler|register_user_hw_breakpoint|register_perf_hw_breakpoint|hw_break_set|modify_user_hw_breakpoint' /proc/kallsyms
  ```

- If `ptrace_resume` is missing, drop the kprobe and rely on the syscall
  tracepoint.
- HW breakpoint handlers will not show up — that's expected, they're
  `NOKPROBE_SYMBOL`.

## 7. Repro results and diagnosis

Single run of the repro from `uprobe-singlestep-repro.md` with the script
edits 1–8 applied. 31 probes attached, no bpftrace runtime errors during
the WARN window (a benign post-run crash in ustack symbolization for an
exiting unrelated `zsh` process happened ~20 s after the workload finished
and is unrelated).

### Counts (this run)

| Metric | Count |
|---|---|
| WARNs in dmesg | 14 |
| `bug_handler_warnpath` events captured | 14 (all on `gpu-burner/749991`) |
| `register_user_hw_breakpoint` calls | **0** |
| `register_perf_hw_breakpoint` calls | **0** |
| `sys_enter_ptrace_singlestep` from gdb against pid 749991 | 33 |
| `uprobe_single_step_handler` total fires | 16370 |
| `uprobe_handle_trampoline` fires | 782 (uretprobes; previously a blind spot) |
| `last_uprobe path` at every WARN | `2` (simulate, `arch_uprobe_skip_sstep` returned 1) |
| Time gap PTRACE_SINGLESTEP → WARN | ~20–80 µs, one-to-one |

### What the added probes proved

- HBP/WP registration probes returned zero. Hardware breakpoints are
  **not** the source. gdb in this configuration uses software single-step.
- The `bug_handler` gate (`@last_uprobe_ns[tid] != 0`) worked: only
  threads with prior uprobe state printed last_uprobe context, no bogus
  zero-aged entries.
- Direct one-to-one timing correlation between `sys_enter_ptrace_singlestep
  target_pid=749991` from `gdb/749913` and `bug_handler_warnpath
  target=gpu-burner/749991`.

### Recovering the faulting uprobe from the log

The WARN's `last_uprobe` line carries `uprobe=0x...`, `auprobe=0x...`,
`bp_vaddr=0x...`, `hit_ns=...`. Match these fields against the same fields
in the corresponding `handler_chain` event for the original uprobe hit;
that event has the resolved `ustack`. For this run:

```text
544196739141650 bug_handler_warnpath target=gpu-burner/749991/749991
  last_uprobe path=2 hit_ns=544196700984066 age_ns=38158757
                  uprobe=0xffff000195713f00 bp_vaddr=0xfffff7c81f50

544196700984796 handler_chain target=gpu-burner/749991/749991
  uprobe=0xffff000195713f00 regs=0xffff8000a242feb0 bp_vaddr=0xfffff7c81f50
  ustack=
    setenv+0
    os_putenv+80
    cfunction_vectorcall_FASTCALL+88
    call_function+136
    _PyEval_EvalFrameDefault+109836
```

The faulting uprobe is on libc's `setenv`. Same `uprobe=0xffff000195713f00`
appears in unrelated processes' (`git`, `ps`, `zsh`, `bpftrace` itself)
handler_chain events too, confirming a single registered system-probe
uprobe is shared across all callers of `setenv`. `auprobe` is at offset
`0xd0` from the `struct uprobe` pointer — the embedded `arch_uprobe`,
matching expected layout.

### Why only `setenv`

Per-uprobe distribution of the `arch_uprobe_skip_sstep` decision on
gpu-burner threads in this run:

| auprobe | hits | path |
|---|---|---|
| `0xffff000183352ad0` (`cuLaunchKernel`) | 2325 | XOL (`ret=0`) |
| `0xffff0004ef8211d0` (`cudaLaunchKernel`) | 1551 | XOL |
| `0xffff00018eb63dd0` | 779 | XOL |
| `0xffff0004ef82e3d0` (`cudaMalloc`) | 3 | XOL |
| `0xffff00018eb659d0` (`cudaSetDevice`) | 1 | XOL |
| **`0xffff000195713fd0` (`setenv`)** | **2** | **simulate (`ret=1`)** |

`setenv`'s first instruction decodes to `INSN_GOOD_NO_SLOT` in
`arm_probe_decode_insn`, so `auprobe->simulate = true` and the kernel
emulates without XOL. Every other uprobe takes XOL.

`setenv` is also the only probed function called during the gdb
single-step storm (Python interpreter startup phase: `os.putenv` →
`setenv`). Cuda* probes only fire later during the matmul workload, by
which time gdb has stopped issuing `PTRACE_SINGLESTEP`. Empirical
windows from this run:

```text
544194786583927  first gdb PTRACE_SINGLESTEP   (start of single-step storm)
544196700944641  gpu-burner hits setenv (simulate, utask now allocated)
544196739141650  first WARN
544199467240298  last gdb PTRACE_SINGLESTEP    (gdb stops)
544199467263658  last WARN                     (immediately after)
544199495312478  first cuda* uprobe XOL fires  (~28 ms after gdb stops)
```

gdb does nothing specific to `setenv`. The single-step storm is gdb's
standard "step over breakpoint" mechanism (likely against the dynamic
linker's `_dl_debug_state` rendezvous breakpoint that fires on every
`dlopen`). Each library load → BP hit → remove BP → `PTRACE_SINGLESTEP`
→ reinsert BP → `PTRACE_CONT`. Python startup dlopens many libraries
(libpython, numpy, torch, CUDA libs), so the storm is dense in that
phase only.

### Diagnosis

Sequence:

1. gpu-burner thread hits the system-probe uprobe on `setenv`.
   `handle_swbp` calls `get_utask()` (allocating `current->utask`), then
   `handler_chain`, then `arch_uprobe_skip_sstep` returns 1 (simulate).
   `pre_ssout` is **not** called, so `utask->xol_vaddr` is never set
   (stays at 0 for a freshly forked child).
2. gdb issues `PTRACE_SINGLESTEP` for the same thread (via its
   step-over-breakpoint dance during `_dl_debug_state` handling).
   `ptrace_resume` → `user_enable_single_step(child)` sets
   `TIF_SINGLESTEP`.
3. Thread returns to userspace, executes one instruction, takes a
   single-step debug exception. `single_step_handler` walks the user
   step-hook list and unconditionally invokes the arm64 uprobe step hook
   `uprobe_single_step_handler`, which executes:

   ```c
   struct uprobe_task *utask = current->utask;
   WARN_ON(utask && (instruction_pointer(regs) != utask->xol_vaddr + 4));
   ```

   `utask` is non-NULL (from step 1), `xol_vaddr` is 0, IP is at user
   code somewhere. The comparison is always false → `WARN_ON` fires.

4. `uprobe_post_sstep_notifier` then returns 0 because it requires
   `utask->active_uprobe` (set only on the XOL path), so
   `single_step_handler` falls through to `send_user_sigtrap(TRAP_TRACE)`
   and gdb gets its expected SIGTRAP. Userspace sees correct behavior;
   the kernel just spurious-WARNs.

This is a kernel bug, not an agent bug. The arm64 `uprobe_single_step_handler`
WARN should also gate on `utask->active_uprobe` (or
`utask->state == UTASK_SSTEP`), since `current->utask` legitimately
exists in non-XOL state for any thread that has ever hit a simulate-path
uprobe (and arguably for any thread that ever hit any uprobe, since
`xol_free_insn_slot` clears `xol_vaddr` to 0 after a successful XOL).

Suggested kernel fix:

```c
WARN_ON(utask && utask->active_uprobe &&
        (instruction_pointer(regs) != utask->xol_vaddr + 4));
```

### Caveats

- The condition is broader than "simulate-only uprobes trip it".
  `xol_free_insn_slot` clears `utask->xol_vaddr`, so any thread that
  completed an XOL'd uprobe will also satisfy `utask != NULL && IP != 4`
  on a subsequent `PTRACE_SINGLESTEP`. This repro doesn't exercise that
  path because gdb's single-step storm finishes before any cuda* XOL
  runs. A repro that issues `PTRACE_SINGLESTEP` after a thread has hit
  any uprobe should also trip the WARN.
- For attribution to system-probe specifically, this trace shows the
  single registered uprobe pointer `0xffff000195713f00` is shared across
  unrelated processes (`git`, `ps`, `zsh`, `bpftrace`) that all call
  `setenv`. That confirms the uprobe is registered against libc's
  `setenv` on this host.
- The `__arm64_sys_perf_event_open` kprobe was removed; the syscall
  tracepoint stayed and showed only bpftrace's own perf opens, no
  user-space HBP/WP setup. Consistent with the
  `register_*_hw_breakpoint` zero counts.
