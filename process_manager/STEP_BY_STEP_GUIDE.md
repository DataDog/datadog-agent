# Step-by-Step Migration Guide

## What We've Done So Far ✅

1. ✅ Created `/process_manager/` directory in datadog-agent repo
2. ✅ Copied all code from your original project:
   - `engine/` - Core library with daemon binary (dd-procmgrd)
   - `cli/` - Command-line client (dd-procmgr)
   - `proto/` - gRPC definitions
   - `tests/` - All tests
   - `examples/` - YAML examples
   - `go-client/` - Go FFI client
3. ✅ Created workspace `Cargo.toml`

## Current Status

Your process manager code is now in the datadog-agent repository at:
```
/Users/josemanuel.almaza/go/src/github.com/DataDog/datadog-agent/process_manager/
```

**Binaries:**
- `dd-procmgrd` - Daemon (in engine/src/bin/daemon.rs)
- `dd-procmgr` - CLI (in cli/src/main.rs)

## Next Steps - Choose Your Path

### Option A: Just Build It (Quick Start)
If you just want to build and test it:

```bash
cd /Users/josemanuel.almaza/go/src/github.com/DataDog/datadog-agent/process_manager

# 1. Build everything
cargo build --release

# 2. Run the daemon
./target/release/dd-procmgrd

# 3. In another terminal, use the CLI
./target/release/dd-procmgr list
```

### Option B: Integrate with Agent Build System (Later)
When you're ready to integrate with the agent's build system:

1. Create invoke tasks in `tasks/process_manager.py`
2. Add to agent's main build
3. Add systemd service files
4. Update documentation

### Option C: Test First
Make sure everything works:

```bash
cd /Users/josemanuel.almaza/go/src/github.com/DataDog/datadog-agent/process_manager

# Run tests
cargo test --workspace

# Run specific test
cargo test -p tests e2e_basic
```

## Suggested First Steps

I recommend:

1. **Today**: Build and verify it works
   ```bash
   cd process_manager
   cargo build --release
   ```

2. **Next**: Create a simple invoke task
   - Add `tasks/process_manager.py` with basic build command
   - Test: `dda inv process-manager.build`

3. **Then**: Document usage for other team members
   - Add README.md specific to datadog-agent context
   - Example configs for managing agent components

4. **Finally**: Production integration
   - Systemd service files
   - CI/CD pipeline
   - Deployment automation

## What Would You Like to Do First?

Let me know which step you'd like to focus on, and I'll help you with just that one thing!

Some options:
- **A**: "Just build it and make sure it compiles"
- **B**: "Create a simple invoke task so I can run `dda inv process-manager.build`"
- **C**: "Write documentation for how to use this"
- **D**: "Something else" (tell me what)

## Quick Reference

**Build commands:**
```bash
# Debug build
cargo build

# Release build
cargo build --release

# Run tests
cargo test

# Build just the daemon
cargo build --bin dd-procmgrd

# Build just the CLI
cargo build --bin dd-procmgr
```

**File locations:**
- Daemon source: `engine/src/bin/daemon.rs`
- CLI source: `cli/src/main.rs`
- Library: `engine/src/lib.rs`
- Tests: `tests/e2e_*.rs`

