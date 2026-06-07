{ pkgs, ... }:

# Cross-compiler toolchains matching the buildimage release artifacts.
#   x86_64-unknown-linux-gnu  targeting glibc 2.17 (amd64 release floor)
#   aarch64-unknown-linux-gnu targeting glibc 2.23 (arm64 release floor)
#
# Built via crosstool-ng 1.26.0 with the same .config files used by the
# Docker buildimage (linux/x86_64/ in datadog-agent-buildimages).
#
# __noChroot = true (TBD-5b stopgap): ctng downloads GCC, glibc, binutils, etc.
# at build time (~700 MB). Replace with CT_LOCAL_TARBALLS_DIR + CT_FORBID_DOWNLOAD=y
# once the tarball manifest is enumerated (see .claude/plans/nix-full.md TBD-5a).
#
# Both toolchains are built in a single derivation so the source tarballs
# downloaded for x86_64 are reused for the aarch64 build (CT_LOCAL_TARBALLS_DIR).
#
# Linux kernel tarballs are pre-fetched by Nix: the nixbld user can reach
# the crosstool-ng.org mirror (which hosts GNU packages) but not cdn.kernel.org,
# so ctng's own wget fails for linux-*.tar.xz. Pre-fetching via pkgs.fetchurl
# puts them in CT_LOCAL_TARBALLS_DIR (= $HOME/src) before ct-ng build runs.
#
# Output layout (matches flake.nix PATH wiring):
#   $out/x86_64/bin/x86_64-unknown-linux-gnu-gcc
#   $out/aarch64/bin/aarch64-unknown-linux-gnu-gcc
#
# Returns { x86_64 = combined; aarch64 = combined; } — both attrs point to the
# same derivation so flake.nix can reference them independently.

let
  ctngVersion = "1.26.0";

  # Tarballs not hosted on crosstool-ng.org/mirrors/ must be pre-fetched here.
  # ctng's CT_GetFile checks CT_LOCAL_TARBALLS_DIR ($HOME/src) before downloading,
  # so placing tarballs there causes ct-ng to use them without any wget attempt.
  # The nixbld build user can reach crosstool-ng.org (GNU mirrors) but not
  # cdn.kernel.org, github.com/madler/zlib, or libisl.sourceforge.io.

  # Linux kernel headers (x86_64 config uses 6.4; aarch64 config uses 6.4.16)
  linux64Tarball = pkgs.fetchurl {
    url = "https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-6.4.tar.xz";
    hash = "sha256-j6BYjwws7KRMrHeg45ukjJ8AprncaXYcAqXT76yNp/M=";
  };

  linux6416Tarball = pkgs.fetchurl {
    url = "https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-6.4.16.tar.xz";
    hash = "sha256-libshKOeywCb8RonHdUglBFZwWXU5i+C46d7edIP8n0=";
  };

  # zlib (companion lib) — from github.com/madler/zlib, not on GNU mirror
  zlibTarball = pkgs.fetchurl {
    url = "https://github.com/madler/zlib/releases/download/v1.2.13/zlib-1.2.13.tar.xz";
    hash = "sha256-0Uw44xOvw1qah2Da3yYEL1HqD10VSwYwox2gVAEH+5g=";
  };

  # ISL (companion lib for GCC polyhedral optimizations) — from sourceforge.io
  islTarball = pkgs.fetchurl {
    url = "https://libisl.sourceforge.io/isl-0.26.tar.xz";
    hash = "sha256-oLXLBtJPn6nne1X6u+mjyUozYZA0XCVV+ZFbs46XZQQ=";
  };

  # zstd (companion lib) — from github.com/facebook/zstd, not on GNU mirror
  zstdTarball = pkgs.fetchurl {
    url = "https://github.com/facebook/zstd/releases/download/v1.5.5/zstd-1.5.5.tar.gz";
    hash = "sha256-nEOWzIKc+uMZpuJhUgLoKq1BNyBzSC/OKG+seGRtPuQ=";
  };

  # expat (companion lib for GDB XML support) — from github.com/libexpat, not on GNU mirror
  expatTarball = pkgs.fetchurl {
    url = "https://github.com/libexpat/libexpat/releases/download/R_2_5_0/expat-2.5.0.tar.xz";
    hash = "sha256-7yQg8CMsCHgBq/cF6JrmX2JX32t5MdN4RqGT7y6M3L4=";
  };

  # Use gcc13Stdenv so the HOST compiler (used to build companion libs like GMP)
  # is GCC 13.x. GMP 6.2.1 (2020) predates GCC 14's stricter defaults
  # (implicit-function-declaration/-int became errors by default); compiling it
  # with GCC 15 causes configure to report "no working compiler". gcc13Stdenv
  # is the latest available pre-strictness stdenv in this nixpkgs (gcc12 was removed).
  combined = pkgs.gcc13Stdenv.mkDerivation {
    name = "dd-cross-toolchains-${ctngVersion}";

    __noChroot = true;

    # Disable all Nix stdenv hardening flags. ctng builds old source trees
    # (GCC 11.4.0, glibc 2.17 from 2012, binutils 2.40) that trigger hardening
    # warnings-as-errors injected by the Nix cc-wrapper — e.g. format-security
    # in gcc/libcpp. These are internal build tools, not shipped binaries.
    hardeningDisable = [ "all" ];

    src = pkgs.fetchurl {
      url = "https://github.com/crosstool-ng/crosstool-ng/releases/download/crosstool-ng-${ctngVersion}/crosstool-ng-${ctngVersion}.tar.xz";
      hash = "sha256-6M5pxcjKjZBOaSPM+GxTV2dhuc8hni5pI1sTnI4bdPw=";
    };

    nativeBuildInputs = with pkgs; [
      # ctng configure + build dependencies
      gperf help2man bison flex texinfo ncurses which
      gnumake autoconf automake libtool rsync unzip
      wget curl pkg-config python3 perl binutils
      # ctng downloads use gawk, sed, patch internally
      # perl: glibc 2.17 manual/stamp-libm-err calls gen-libm-err.pl
      gawk gnused patch gettext
      # archive tools for source tarballs
      gzip bzip2 xz
      # file identification
      file
    ];

    # ctng.patch: adds gnu.mirror.constant.com as the first GNU mirror.
    # ctng-gmp-cpp-for-build.patch: fixes typo in ctng 1.26.0 where
    #   CPP_FOR_BUILD="{CT_BUILD}-cpp" (missing $) passes a literal instead of
    #   the expanded build triplet as the preprocessor when configuring GMP.
    patches = [
      ./ctng-configs/ctng.patch
      ./ctng-configs/ctng-gmp-cpp-for-build.patch
    ];

    buildPhase = ''
      echo "=== PRE-BUILD DIAGNOSTICS ==="
      echo "uname -m: $(uname -m)"
      echo "gcc path: $(which gcc)"
      echo "gcc --version: $(gcc --version | head -1)"
      echo "gcc -dumpmachine: $(gcc -dumpmachine)"
      echo "=== END DIAGNOSTICS ==="

      # Build ct-ng itself in local mode (no install step needed)
      ./configure --enable-local
      make -j$NIX_BUILD_CORES

      # HOME must be writable: ctng writes x-tools/ output and src/ tarballs cache here.
      # The .config files set CT_LOCAL_TARBALLS_DIR to HOME/src.
      export HOME=$TMPDIR
      mkdir -p $HOME/src

      # Pre-seed CT_LOCAL_TARBALLS_DIR with linux kernel tarballs.
      # ctng's CT_GetFile checks CT_LOCAL_TARBALLS_DIR before downloading;
      # finding the tarball there causes it to move/link it in and skip the
      # wget entirely. This is needed because cdn.kernel.org is inaccessible
      # from the nixbld build user even with __noChroot = true.
      cp ${linux64Tarball}   $HOME/src/linux-6.4.tar.xz
      cp ${linux6416Tarball} $HOME/src/linux-6.4.16.tar.xz
      cp ${zlibTarball}      $HOME/src/zlib-1.2.13.tar.xz
      cp ${islTarball}       $HOME/src/isl-0.26.tar.xz
      cp ${zstdTarball}      $HOME/src/zstd-1.5.5.tar.gz
      cp ${expatTarball}     $HOME/src/expat-2.5.0.tar.xz

      # Needed when building as root (no-op for non-root Nix build users).
      export CT_ALLOW_BUILD_AS_ROOT_SURE=yes

      # ctng aborts if CC or AR are set externally ("Don't set CC. It screws up
      # the build."). Unset the vars ctng checks for.
      #
      # ctng auto-detects the build host compiler by searching PATH for
      # CT_BUILD-gcc (= aarch64-unknown-linux-gnu-gcc on this host). That name
      # resolves to the RAW gcc binary (not the Nix cc-wrapper), which lacks the
      # wrapper's -B glibc path injection — so Scrt1.o / crti.o are not found
      # and the trivial program sanity check fails.
      #
      # Fix: put shim scripts named aarch64-unknown-linux-gnu-{gcc,g++} first in
      # PATH. The shims forward to plain "gcc"/"g++" (the Nix wrapper). ctng finds
      # the shims, symlinks to them, and all host-compiler invocations go through
      # the wrapper with correct glibc paths. Shims must be real shell scripts —
      # symlinks would be dereffed back to the raw binary.
      unset CC CXX CPP
      mkdir -p $TMPDIR/cc-shims

      # Capture absolute Nix-store paths NOW, before ctng modifies PATH.
      # ctng adds .build/tools/bin and .build/buildtools/bin to PATH during
      # the build; these can contain gcc/binutils stubs that shadow the real
      # tools. By baking the absolute path into each shim we ensure every
      # invocation uses the correct Nix-wrapped binary regardless of PATH.
      _gcc=$(which gcc)
      _gxx=$(which g++)
      _cpp=$(which cpp)
      _ar=$(which ar)
      _as=$(which as)
      _nm=$(which nm)
      _ranlib=$(which ranlib)
      _strip=$(which strip)
      _objcopy=$(which objcopy)
      _objdump=$(which objdump)
      _ld=$(which ld)

      # Create shims for BOTH the HOST triplet and ctng's internal BUILD triplet
      # (which ctng denotes as aarch64-build_unknown-linux-gnu with "build_" inserted).
      # The -cpp shim is needed because ctng passes CPP_FOR_BUILD="<CT_BUILD>-cpp"
      # to GMP's configure (after the typo-fix patch); GMP's configure will probe it.
      for triplet in aarch64-unknown-linux-gnu aarch64-build_unknown-linux-gnu; do
        printf '#!/bin/sh\nexec %s "$@"\n' "$_gcc"    > $TMPDIR/cc-shims/$triplet-gcc
        printf '#!/bin/sh\nexec %s "$@"\n' "$_gxx"    > $TMPDIR/cc-shims/$triplet-g++
        printf '#!/bin/sh\nexec %s "$@"\n' "$_cpp"    > $TMPDIR/cc-shims/$triplet-cpp
        printf '#!/bin/sh\nexec %s "$@"\n' "$_ar"     > $TMPDIR/cc-shims/$triplet-ar
        printf '#!/bin/sh\nexec %s "$@"\n' "$_as"     > $TMPDIR/cc-shims/$triplet-as
        printf '#!/bin/sh\nexec %s "$@"\n' "$_nm"     > $TMPDIR/cc-shims/$triplet-nm
        printf '#!/bin/sh\nexec %s "$@"\n' "$_ranlib" > $TMPDIR/cc-shims/$triplet-ranlib
        printf '#!/bin/sh\nexec %s "$@"\n' "$_strip"  > $TMPDIR/cc-shims/$triplet-strip
        printf '#!/bin/sh\nexec %s "$@"\n' "$_objcopy"> $TMPDIR/cc-shims/$triplet-objcopy
        printf '#!/bin/sh\nexec %s "$@"\n' "$_objdump"> $TMPDIR/cc-shims/$triplet-objdump
        printf '#!/bin/sh\nexec %s "$@"\n' "$_ld"     > $TMPDIR/cc-shims/$triplet-ld
        chmod +x $TMPDIR/cc-shims/$triplet-*
      done

      export PATH=$TMPDIR/cc-shims:$PATH

      # ── x86_64-unknown-linux-gnu (glibc 2.17) ────────────────────────────
      cp ${./ctng-configs/config-x86_64-unknown-gnu-linux-glibc2.17} .config
      ./ct-ng upgradeconfig
      ./ct-ng build CT_JOBS=$NIX_BUILD_CORES || {
        echo "=== CTNG FAILURE (x86_64) ==="
        echo "--- last 60 ERROR lines from build.log ---"
        grep -E "\[ERROR\]" build.log 2>/dev/null | tail -60 || echo "(no build.log)"
        echo "=== END ==="
        exit 1
      }

      # ── aarch64-unknown-linux-gnu (glibc 2.23) ────────────────────────────
      # $HOME/src now contains the GCC/glibc/binutils tarballs from the first
      # build; ctng will reuse them instead of re-downloading.
      #
      # upgradeconfig writes .config.before-upgrade/-olddefconfig/.old as
      # read-only files; remove them so the second run can create fresh ones.
      rm -f .config.before-upgrade .config.before-olddefconfig .config.old
      cp ${./ctng-configs/config-aarch64-unknown-gnu-linux-glibc2.23} .config
      ./ct-ng upgradeconfig
      ./ct-ng build CT_JOBS=$NIX_BUILD_CORES || {
        echo "=== CTNG FAILURE (aarch64) ==="
        echo "--- last 60 ERROR lines from build.log ---"
        grep -E "\[ERROR\]" build.log 2>/dev/null | tail -60 || echo "(no build.log)"
        echo "=== END ==="
        exit 1
      }
    '';

    installPhase = ''
      mkdir -p $out/x86_64 $out/aarch64
      cp -r $TMPDIR/x-tools/x86_64-unknown-linux-gnu/. $out/x86_64/
      cp -r $TMPDIR/x-tools/aarch64-unknown-linux-gnu/. $out/aarch64/
    '';

    meta = {
      description = "Datadog Agent release cross-compilers (ctng ${ctngVersion})";
      longDescription = ''
        x86_64-unknown-linux-gnu targeting glibc 2.17 (agent release floor for amd64)
        aarch64-unknown-linux-gnu targeting glibc 2.23 (agent release floor for arm64)
        Matches the toolchains baked into the CI buildimage.
      '';
      platforms = [ "x86_64-linux" "aarch64-linux" ];
    };
  };

in {
  x86_64  = combined;
  aarch64 = combined;
}
