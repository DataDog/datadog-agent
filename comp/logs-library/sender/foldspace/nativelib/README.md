# Vendored libfoldspace_go

Prebuilt shared objects for the native foldspace library, linked by
`core_native.go` under the `foldspace` build tag.

Only compiled artifacts and the C header (`../foldspace_go.h`) live here. The
library's own sources stay in its repository.

## Provenance

| | |
|---|---|
| Source | `DataDog/foldspace`, crate `bindings/go/native` (`foldspace-go-ffi`) |
| Commit | `a2e956956b121dc4574bd99c647e149c8312016c` |
| Rust | 1.94.0, per the library's `rust-toolchain.toml` |
| ABI | 1, as asserted against `FOLDSPACE_ABI_VERSION` at construction |

| Platform | sha256 |
|---|---|
| `linux_amd64/libfoldspace_go.so` | `2fac8ea5971684ab7b9e9d03c2ff7aa3a9c82e0845e4972a70f2f85662b7a81b` |
| `linux_arm64/libfoldspace_go.so` | `6dc60c151f2c93820d61050a3656da35fcdf28c00b4d983167e0692b36b36173` |

Platforms without a binary here cannot build the `foldspace` tag from this
directory. Windows and AIX exclude the tag in `tasks/build_tags.bzl`. Darwin
does not: it links a locally produced library through `CGO_LDFLAGS`.

## Regenerating

The header and the library are one artifact: the header declares the ABI a
particular revision exports, and a mismatch is caught at construction rather
than at link time. Take both from the same commit.

Both arches build in an `arm64` container. `amd64` is cross-compiled rather
than emulated, because QEMU crashes gcc while building zstd's C sources. The
paths are under `$HOME` because that is the one directory every macOS Docker
VM shares by default; Colima shares nothing else.

```bash
git -C <foldspace> worktree add ~/fs-pin a2e956956b121dc4574bd99c647e149c8312016c

docker run --rm --platform linux/arm64 \
  -v ~/fs-pin:/src:ro -v ~/fs-out:/out -w /src \
  -e CARGO_TARGET_DIR=/out/target \
  rust:1.94.0 bash -c '
    apt-get update -qq && apt-get install -y -qq protobuf-compiler gcc-x86-64-linux-gnu
    rustup target add x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu
    export CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_LINKER=x86_64-linux-gnu-gcc
    export CC_x86_64_unknown_linux_gnu=x86_64-linux-gnu-gcc
    # A cdylib carries no SONAME by default, which leaves the linker recording
    # whatever path it resolved at build time.
    export RUSTFLAGS="-C link-arg=-Wl,-soname,libfoldspace_go.so"
    for target in x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu; do
      cargo build --release --target "$target" -p foldspace-go-ffi
    done'
```

Copy `libfoldspace_go.so` from each target's `release/` directory into the
matching directory here, and update the header, the commit and the checksums
above.
