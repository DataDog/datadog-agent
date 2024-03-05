# EBPF Instrumentation

EBPF instrumentation refers to the process of attaching hooks in the beginning of eBPF bytecode. These hooks serve as trampolines to bytecode intended to be run before the actual eBPF program.

## Trampoline
A trampoline is an absolute jump to the end of the bytecode sequence.
A trampoline instruction is instrumented in the bytecode at compile time by specifying `-pg` as an [argument](https://clang.llvm.org/docs/ClangCommandLineReference.html#cmdoption-clang-pg) to clang. This is a profiling options which instructs the compiler to instrument a call to a function called `mcount` in the
beginning of each function. Since this functions is not present in eBPF, the compiler instead instruments `call -1`. This adds as our marker to patch the trampoline.
We leverage the fixed instruction size architecture of eBPF to replace the `call -1` with a `ja <END>` instruction, where `<END>` is the end of the bytecode.
