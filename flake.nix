{
  description = "Datadog Agent development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    rust-overlay = {
      url = "github:oxalica/rust-overlay";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, flake-utils, rust-overlay }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        overlays = [ (import rust-overlay) ];
        pkgs = import nixpkgs { inherit system overlays; };

        # Read versions from the repo's own version files so flake.lock is the
        # source of provenance while the repo files remain the source of versions.
        goVersion  = pkgs.lib.strings.trim (builtins.readFile ./.go-version);
        pyVersionRaw = pkgs.lib.strings.trim (builtins.readFile ./.python-version);

        # Go: resolve the go_X_Y package attribute from the version string.
        # e.g. "1.25.10" -> pkgs.go_1_25 (nixpkgs names by major.minor only)
        goMajorMinor =
          let parts = pkgs.lib.strings.splitString "." goVersion;
          in "go_${builtins.elemAt parts 0}_${builtins.elemAt parts 1}";
        goPkg = pkgs.${goMajorMinor};

        # Rust: use rust-overlay to honour rust-toolchain.toml (which has
        # components but needs a channel — see rust-toolchain.toml).
        rustPkg = pkgs.rust-bin.fromRustupToolchainFile ./rust-toolchain.toml;

        # Python: resolve python3XX from major.minor
        pyAttr =
          let parts = pkgs.lib.strings.splitString "." pyVersionRaw;
          in "python${builtins.elemAt parts 0}${builtins.elemAt parts 1}";
        pythonPkg = pkgs.${pyAttr};

        # dda version (read from .dda/version)
        ddaVersion = pkgs.lib.strings.trim (builtins.readFile ./.dda/version);

      in {
        devShells.default = pkgs.mkShell {
          name = "datadog-agent";

          buildInputs = [
            # --- Language toolchains ---
            goPkg
            rustPkg
            pythonPkg
            pythonPkg.pkgs.setuptools   # needed by some cgo builds
            # Ruby 2.7 is EOL and not in nixpkgs; use 3.3 (stable LTS).
            # The omnibus Gemfile has no hard Ruby version constraint.
            # Ruby 2.7 is EOL and not in nixpkgs; use 3.3 (stable LTS).
            # The omnibus Gemfile has no hard Ruby version constraint.
            pkgs.ruby_3_3
            pkgs.bundler

            # --- Host C/C++ toolchain (for rtloader CMake build) ---
            pkgs.stdenv.cc
            pkgs.cmake
            pkgs.gnumake
            pkgs.pkg-config

            # --- System libraries (cgo + omnibus linker deps) ---
            pkgs.openssl
            pkgs.openssl.dev
            pkgs.zlib
            pkgs.zlib.dev
            pkgs.libffi
            pkgs.libffi.dev
            # systemd dev headers: required by go-systemd (coreos/go-systemd) for
            # linter typechecking; agent.build excludes systemd but linter sees all.
            pkgs.systemd.dev

            # --- uv (for dda install) ---
            pkgs.uv

            # --- Misc tools ---
            pkgs.git
            pkgs.curl
            pkgs.cacert
            pkgs.coreutils
            pkgs.patchelf    # rtloader RPATH + omnibus rpath_edit (Linux)
            pkgs.which

            # --- Nix builds may need bzip2 + xz for Go toolchain download ---
            pkgs.bzip2
            pkgs.xz
          ];

          shellHook = ''
            # ----------------------------------------------------------------
            # Writable per-repo tool directories so go install / cargo install
            # / bundle install work as non-root without touching /nix/store.
            # ----------------------------------------------------------------

            # Override TMPDIR: Nix sets it to /tmp/nix-shell.XXXX which produces
            # very long paths that exceed Linux's 108-char Unix socket path limit,
            # breaking tests that create sockets via t.TempDir().
            export TMPDIR=/tmp

            export GOBIN="$PWD/.gobin"
            export GOMODCACHE="$PWD/.gomodcache"
            export GOPATH="$PWD/.gopath"
            export CARGO_HOME="$PWD/.cargo-home"
            export GEM_HOME="$PWD/.gem"
            export BUNDLE_PATH="$PWD/.bundle"
            export PATH="$GOBIN:$CARGO_HOME/bin:$GEM_HOME/bin:$PATH"

            # SSL certs for curl / git / pip
            export SSL_CERT_FILE="${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
            export NIX_SSL_CERT_FILE="${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
            export GIT_SSL_CAINFO="${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"

            # rtloader CMake hint: point at the Nix Python prefix so
            # rtloader/three/CMakeLists.txt can find headers without Conda.
            export DD_RTLOADER_PYTHON3_ROOT="${pythonPkg}"

            # C library hints for cgo
            export PKG_CONFIG_PATH="${pkgs.openssl.dev}/lib/pkgconfig:${pkgs.zlib.dev}/lib/pkgconfig:${pkgs.libffi.dev}/lib/pkgconfig"

            # Install dda if not already present at the pinned version
            DDA_BIN="$HOME/.local/bin/dda"
            if ! command -v dda &>/dev/null || ! dda --version 2>/dev/null | grep -q "${ddaVersion}"; then
              echo "[nix] installing dda==${ddaVersion}..."
              uv tool install --quiet "dda==${ddaVersion}" || true
              export PATH="$HOME/.local/bin:$PATH"
            fi

            echo "✓ Nix dev shell ready"
            echo "  Go:     $(go version 2>/dev/null | cut -d' ' -f3)"
            echo "  Rust:   $(rustc --version 2>/dev/null | cut -d' ' -f2)"
            echo "  Python: $(python3 --version 2>/dev/null)"
            echo "  Ruby:   $(ruby --version 2>/dev/null | cut -d' ' -f2)"
            echo "  GOBIN:  $GOBIN"
          '';
        };
      }
    );
}
