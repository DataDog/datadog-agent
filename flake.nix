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
        pkgs = import nixpkgs {
          inherit system overlays;
          # openssl_1_1 is EOL but required by Ruby 2.7's bundled openssl
          # extension (ext/openssl/extconf.rb hard-rejects OpenSSL >= 3.0.0).
          config.permittedInsecurePackages = [ "openssl-1.1.1w" ];
        };
        # Ruby 2.7.8 built from source using the nixpkgs-unstable stdenv.
        # We do NOT use nixpkgs-ruby27 (nixos-23.11) on any platform: that
        # snapshot's Darwin bootstrap chain requires building LLVM 16, which
        # fails on macOS 26 (Darwin 25.x / Tahoe).  Building against
        # nixpkgs-unstable's toolchain sidesteps this on both Darwin and Linux.
        # Bundler 2.4.22 is pre-installed in the derivation (see nix/ruby27.nix).
        ruby27Pkg  = import ./nix/ruby27.nix { inherit pkgs; };
        rubyPkg    = ruby27Pkg;
        bundlerPkg = ruby27Pkg;  # bundler lives in ruby27Pkg's bin/

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
          # GOMODCACHE and GEM_HOME live outside the repo so omnibus PathFetcher
          # (which copies the entire source tree) does not encounter read-only
          # module cache paths or socket files (e.g. git fsmonitor--daemon.ipc
          # inside bundler gem .git dirs) that it cannot handle.
          export GOMODCACHE="$HOME/.cache/go/pkg/mod"
          export GOPATH="$PWD/.gopath"
          export CARGO_HOME="$PWD/.cargo-home"
          # Version GEM_HOME by Ruby ABI (e.g. 2.7.0) so gems installed by one
          # Ruby version never conflict with a different version's gem home.
          _RUBY_ABI="$(ruby -e 'puts RbConfig::CONFIG["ruby_version"]' 2>/dev/null || echo unknown)"
          export GEM_HOME="$HOME/.gem/ruby/$_RUBY_ABI"
          export BUNDLE_PATH="$HOME/.bundle/ruby/$_RUBY_ABI"
          export PATH="$GOBIN:$CARGO_HOME/bin:$GEM_HOME/bin:$PATH"

          # SSL certs for curl / git / pip
          export SSL_CERT_FILE="${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
          export NIX_SSL_CERT_FILE="${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
          export GIT_SSL_CAINFO="${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"

          # Omnibus finalize: redirect system paths that require root so a
          # non-root local build can succeed.  CI runs as root and leaves
          # these unset, falling back to the real system paths.
          export OUTPUT_CONFIG_DIR="''${OUTPUT_CONFIG_DIR:-$TMPDIR/omnibus-output-config}"
          export DD_LOG_DIR="''${DD_LOG_DIR:-$TMPDIR/omnibus-log}"
          export DD_SYS_BIN_DIR="''${DD_SYS_BIN_DIR:-$TMPDIR/omnibus-bin}"
          mkdir -p "$OUTPUT_CONFIG_DIR" "$DD_LOG_DIR" "$DD_SYS_BIN_DIR"

          # Omnibus base/cache dirs default to /opt/omnibus and /var/cache/omnibus
          # which require root.  Redirect to $HOME/.omnibus/* for non-root dev builds.
          # CI overrides these (or runs as root) so setting them here is safe.
          export OMNIBUS_BASE_DIR="''${OMNIBUS_BASE_DIR:-$HOME/.omnibus/base}"
          export OMNIBUS_CACHE_DIR="''${OMNIBUS_CACHE_DIR:-$HOME/.omnibus/cache}"
          # INSTALL_DIR is read by omnibus/config/projects/agent.rb.
          # Default /opt/datadog-agent requires root; redirect to a writable path.
          export INSTALL_DIR="''${INSTALL_DIR:-$HOME/.omnibus/opt/datadog-agent}"
          mkdir -p "$OMNIBUS_BASE_DIR" "$OMNIBUS_CACHE_DIR" "$INSTALL_DIR"

          # macOS git fsmonitor daemon leaves a Unix socket at .git/fsmonitor--daemon.ipc.
          # omnibus file_syncer doesn't handle socket files and raises RuntimeError.
          # Stop the daemon now (it restarts on-demand); omnibus/config/software/datadog-agent.rb
          # also excludes the socket via its `exclude:` list for belt-and-suspenders coverage.
          git fsmonitor--daemon stop 2>/dev/null || true

          # Shell-local gitconfig: strip the HTTPS->SSH redirect so bundler
          # can clone gem sources (omnibus-ruby, etc.) from GitHub via HTTPS.
          # Datadog developer setups typically have:
          #   url."git@github.com:".insteadOf = https://github.com/
          # which breaks bundler when no SSH keys are forwarded to the nix env.
          # All other settings (user identity, signing, hooks) are preserved.
          _NIX_GITCFG="$TMPDIR/nix-shell-gitconfig"
          awk '
            /^\[url "git@github\.com:"\]/ { skip=1; next }
            /^\[/ { skip=0 }
            !skip
          ' "$HOME/.gitconfig" > "$_NIX_GITCFG" 2>/dev/null || true
          [ -s "$_NIX_GITCFG" ] && export GIT_CONFIG_GLOBAL="$_NIX_GITCFG"

          # rtloader CMake hint: point at the Nix Python prefix so
          # rtloader/three/CMakeLists.txt can find headers without Conda.
          export DD_RTLOADER_PYTHON3_ROOT="${pythonPkg}"

          # C library hints for cgo
          export PKG_CONFIG_PATH="${pkgs.openssl.dev}/lib/pkgconfig:${pkgs.zlib.dev}/lib/pkgconfig:${pkgs.libffi.dev}/lib/pkgconfig"

          # gopacket/pcap needs pcap.h (compile) and libpcap.dylib (link).
          # omnibus datadog-agent.rb overrides CGO_CFLAGS/CGO_LDFLAGS and the Nix clang
          # wrapper's paths are lost; NIX_LDFLAGS is stripped by omnibus replace_env.
          # Export explicit paths forwarded via omnibus.py ENV_PASSTHROUGH so datadog-agent.rb
          # can append them.  Symlinks into embedded/ are belt-and-suspenders for manual builds
          # that bypass dda inv (where ENV_PASSTHROUGH filtering does not apply).
          export DD_LIBPCAP_INCLUDE="${pkgs.libpcap}/include"
          export DD_LIBPCAP_LIB="${pkgs.libpcap.lib}/lib"
          mkdir -p "$INSTALL_DIR/embedded/include"
          ln -sf "${pkgs.libpcap}/include/pcap.h" "$INSTALL_DIR/embedded/include/pcap.h" 2>/dev/null || true
          ln -sf "${pkgs.libpcap}/include/pcap"   "$INSTALL_DIR/embedded/include/pcap"   2>/dev/null || true

          # Go's net package links against -lresolv; on Nix Darwin the library lives in the Nix
          # store (NIX_LDFLAGS), not in the macOS SDK, and NIX_LDFLAGS is stripped by omnibus's
          # replace_env=True.  Extract the lib path once at shell-enter time and re-export so
          # datadog-agent.rb can append -L to CGO_LDFLAGS via ENV_PASSTHROUGH.
          _resolv_lib=$(echo "$NIX_LDFLAGS" | tr ' ' '\n' | grep '^-L.*libresolv' | head -1 | sed 's/^-L//')
          [ -n "$_resolv_lib" ] && export DD_NIX_LIBRESOLV_LIB="$_resolv_lib"
          unset _resolv_lib

          # macOS: the Nix apple-sdk-14.4 clang wrapper reads MACOSX_DEPLOYMENT_TARGET_arm64_apple_darwin
          # (triple-suffixed) in add-flags.sh to embed the minimum OS version in binaries.  The generic
          # MACOSX_DEPLOYMENT_TARGET is only consulted when NIX_CC_WRAPPER_TARGET_HOST_arm64_apple_darwin
          # is set — which omnibus replace_env strips.  We create a thin shim in PATH that pre-sets the
          # triple-suffixed var to 12.0 before the real Nix clang wrapper reads it.  Since omnibus
          # passes PATH through ENV_PASSTHROUGH, the Go CGO build (inside omnibus) picks up the shim.
          # The finalize check requires all binaries have minimum macOS version ≤ 12.0.
          if [[ "$(uname -s)" == "Darwin" ]]; then
            _cc_shim_dir="$TMPDIR/nix-cc-macos12/bin"
            mkdir -p "$_cc_shim_dir"
            for _cc_name in clang clang++ cc c++; do
              _cc_real="$(type -P "$_cc_name" 2>/dev/null)" || continue
              printf '#!/bin/bash\nexport MACOSX_DEPLOYMENT_TARGET_arm64_apple_darwin=12.0\nexec "%s" "$@"\n' \
                "$_cc_real" > "$_cc_shim_dir/$_cc_name"
              chmod +x "$_cc_shim_dir/$_cc_name"
            done
            export PATH="$_cc_shim_dir:$PATH"
            unset _cc_shim_dir _cc_name _cc_real
          fi

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
          echo "  Ruby:   $(ruby --version 2>/dev/null | cut -d' ' -f2) (2.7.8 built from source)"
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
            rubyPkg
            bundlerPkg

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
            pkgs.libpcap   # gopacket/pcap requires pcap.h (systemprobechecks build tag)

            # --- uv (for dda install) ---
            pkgs.uv

            # --- Misc tools ---
            pkgs.git
            pkgs.curl
            pkgs.cacert
            pkgs.coreutils
            pkgs.which
            pkgs.gotestsum

            # --- Nix builds may need bzip2 + xz for Go toolchain download ---
            pkgs.bzip2
            pkgs.xz
          ] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
            # go-systemd headers for linter: all imports are guarded by
            # //go:build systemd; build_tags.py auto-strips the tag on Darwin.
            pkgs.systemd.dev
            # ELF RPATH rewriting; macOS uses install_name_tool from the SDK.
            pkgs.patchelf
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
            rubyPkg bundlerPkg
            pkgs.stdenv.cc pkgs.cmake pkgs.gnumake pkgs.pkg-config
            pkgs.openssl pkgs.openssl.dev pkgs.zlib pkgs.zlib.dev
            pkgs.libffi pkgs.libffi.dev pkgs.libpcap
            pkgs.uv pkgs.git pkgs.curl pkgs.cacert pkgs.coreutils
            pkgs.which pkgs.bzip2 pkgs.xz pkgs.gotestsum
          ] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
            pkgs.systemd.dev
            pkgs.patchelf
          ] ++
            pkgs.lib.optionals (crossToolchainsPkg != null && pkgs.stdenv.isLinux) [
              crossToolchainsPkg.x86_64
              crossToolchainsPkg.aarch64
            ] ++
            pkgs.lib.optionals (embeddedPythonPkg != null && pkgs.stdenv.isLinux) [ embeddedPythonPkg ];
          shellHook = commonShellHook + ''
            # --- Release toolchain additions ---
            ${pkgs.lib.optionalString (crossToolchainsPkg != null && pkgs.stdenv.isLinux) ''
              export PATH="${crossToolchainsPkg.x86_64}/x86_64/bin:${crossToolchainsPkg.aarch64}/aarch64/bin:$PATH"
            ''}
            ${pkgs.lib.optionalString (embeddedPythonPkg != null && pkgs.stdenv.isLinux) ''
              export EMBEDDED_PYTHON="${embeddedPythonPkg}"
              export PYTHON_HOME_3="${embeddedPythonPkg}"
              # Override the dev-shell rtloader hint so CMake uses the release Python.
              export DD_RTLOADER_PYTHON3_ROOT="${embeddedPythonPkg}"
            ''}
          '';
        };
      }
    );
}
