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
        goVersion    = pkgs.lib.strings.trim (builtins.readFile ./.go-version);
        pyVersionRaw = pkgs.lib.strings.trim (builtins.readFile ./.python-version);

        # Go: e.g. "1.25.10" -> pkgs.go_1_25
        goMajorMinor =
          let parts = pkgs.lib.strings.splitString "." goVersion;
          in "go_${builtins.elemAt parts 0}_${builtins.elemAt parts 1}";
        goPkg = pkgs.${goMajorMinor};

        # Rust: honour rust-toolchain.toml (needs channel line — see rust-toolchain.toml)
        rustPkg = pkgs.rust-bin.fromRustupToolchainFile ./rust-toolchain.toml;

        # Python dev shell: e.g. "3.12" -> pkgs.python312
        pyAttr =
          let parts = pkgs.lib.strings.splitString "." pyVersionRaw;
          in "python${builtins.elemAt parts 0}${builtins.elemAt parts 1}";
        pythonPkg = pkgs.${pyAttr};

        # dda version (read from .dda/version)
        ddaVersion = pkgs.lib.strings.trim (builtins.readFile ./.dda/version);

        # Release toolchain packages — implemented in nix/embedded-python.nix and
        # nix/cross-toolchains.nix (see .claude/plans/nix-full.md §2 and §3).
        # Null until those files exist; devShells.release degrades gracefully.
        embeddedPythonPkg = if builtins.pathExists ./nix/embedded-python.nix
          then import ./nix/embedded-python.nix { inherit pkgs; }
          else null;
        crossToolchainsPkg = if builtins.pathExists ./nix/cross-toolchains.nix
          then import ./nix/cross-toolchains.nix { inherit pkgs; }
          else null;

        # Shared shellHook — extracted so devShells.release can concatenate it
        # without a self-referential forward reference.
        commonShellHook = ''
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
            # systemd dev headers: required by go-systemd for linter typechecking;
            # agent.build excludes systemd but linter sees all packages.
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

          shellHook = commonShellHook;
        };

        # devShells.release — extends default with the release toolchain:
        #   - glibc-targeting cross-compilers (nix/cross-toolchains.nix, TBD)
        #   - Nix-built embedded Python (nix/embedded-python.nix, TBD)
        #
        # Usage:
        #   nix develop .#release
        #   dda inv agent.build    # produces a glibc-2.17-floor binary
        #   dda inv omnibus.build  # release artifact with Nix-built embedded Python
        #
        # Until nix/cross-toolchains.nix and nix/embedded-python.nix are written,
        # this shell is identical to devShells.default but prints a warning.
        # See .claude/plans/nix-full.md §1b and §3/§2 for the implementation plan.
        devShells.release = pkgs.mkShell {
          name = "datadog-agent-release";
          # Inherit the full default toolchain set, then add release-specific packages.
          # inputsFrom cannot reference devShells.default (non-rec attrset), so we
          # list the default buildInputs inline and append the release toolchain.
          buildInputs = [
            goPkg rustPkg pythonPkg pythonPkg.pkgs.setuptools
            pkgs.ruby_3_3 pkgs.bundler
            pkgs.stdenv.cc pkgs.cmake pkgs.gnumake pkgs.pkg-config
            pkgs.openssl pkgs.openssl.dev pkgs.zlib pkgs.zlib.dev
            pkgs.libffi pkgs.libffi.dev pkgs.systemd.dev
            pkgs.uv pkgs.git pkgs.curl pkgs.cacert pkgs.coreutils
            pkgs.patchelf pkgs.which pkgs.bzip2 pkgs.xz
          ] ++
            pkgs.lib.optionals (crossToolchainsPkg != null) [
              crossToolchainsPkg.x86_64
              crossToolchainsPkg.aarch64
            ] ++
            pkgs.lib.optionals (embeddedPythonPkg != null) [ embeddedPythonPkg ];
          shellHook = commonShellHook + ''
            # --- Release toolchain additions ---
            ${pkgs.lib.optionalString (crossToolchainsPkg != null) ''
              export PATH="${crossToolchainsPkg.x86_64}/x86_64/bin:${crossToolchainsPkg.aarch64}/aarch64/bin:$PATH"
            ''}
            ${pkgs.lib.optionalString (embeddedPythonPkg != null) ''
              export EMBEDDED_PYTHON="${embeddedPythonPkg}"
              export PYTHON_HOME_3="${embeddedPythonPkg}"
            ''}
            ${pkgs.lib.optionalString (crossToolchainsPkg == null || embeddedPythonPkg == null) ''
              echo "  ⚠  Release toolchain not yet built (see .claude/plans/nix-full.md TBD-3,5,6)"
              echo "     Cross-compilers: ${if crossToolchainsPkg != null then "available" else "pending nix/cross-toolchains.nix"}"
              echo "     Embedded Python: ${if embeddedPythonPkg != null then "available" else "pending nix/embedded-python.nix"}"
              echo "     Binaries will link against host glibc until cross-compilers are available."
            ''}
          '';
        };
      }
    );
}
