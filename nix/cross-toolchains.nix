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
# Output layout (matches flake.nix PATH wiring):
#   $out/x86_64/bin/x86_64-unknown-linux-gnu-gcc
#   $out/aarch64/bin/aarch64-unknown-linux-gnu-gcc
#
# Returns { x86_64 = combined; aarch64 = combined; } — both attrs point to the
# same derivation so flake.nix can reference them independently.

let
  ctngVersion = "1.26.0";

  combined = pkgs.stdenv.mkDerivation {
    name = "dd-cross-toolchains-${ctngVersion}";

    __noChroot = true;

    src = pkgs.fetchurl {
      url = "https://github.com/crosstool-ng/crosstool-ng/releases/download/crosstool-ng-${ctngVersion}/crosstool-ng-${ctngVersion}.tar.xz";
      hash = "sha256-6M5pxcjKjZBOaSPM+GxTV2dhuc8hni5pI1sTnI4bdPw=";
    };

    nativeBuildInputs = with pkgs; [
      # ctng configure + build dependencies
      gperf help2man bison flex texinfo ncurses which
      gnumake autoconf automake libtool rsync unzip
      wget curl pkg-config python3 gcc binutils
      # ctng downloads use gawk, sed, patch internally
      gawk gnused patch gettext
      # archive tools for source tarballs
      gzip bzip2 xz
      # file identification
      file
    ];

    # Adds gnu.mirror.constant.com as the first GNU mirror — copied from
    # linux/ctng.patch in datadog-agent-buildimages.
    patches = [ ./ctng-configs/ctng.patch ];

    buildPhase = ''
      # Build ct-ng itself in local mode (no install step needed)
      ./configure --enable-local
      make -j$NIX_BUILD_CORES

      # HOME must be writable: ctng writes x-tools/ output and src/ tarballs cache here.
      # The .config files set CT_LOCAL_TARBALLS_DIR to HOME/src.
      export HOME=$TMPDIR
      mkdir -p $HOME/src

      # Needed when building as root (no-op for non-root Nix build users).
      export CT_ALLOW_BUILD_AS_ROOT_SURE=yes

      # ── x86_64-unknown-linux-gnu (glibc 2.17) ────────────────────────────
      cp ${./ctng-configs/config-x86_64-unknown-gnu-linux-glibc2.17} .config
      ./ct-ng upgradeconfig
      ./ct-ng build CT_JOBS=$NIX_BUILD_CORES

      # ── aarch64-unknown-linux-gnu (glibc 2.23) ────────────────────────────
      # $HOME/src now contains the GCC/glibc/binutils tarballs from the first
      # build; ctng will reuse them instead of re-downloading.
      cp ${./ctng-configs/config-aarch64-unknown-gnu-linux-glibc2.23} .config
      ./ct-ng upgradeconfig
      ./ct-ng build CT_JOBS=$NIX_BUILD_CORES
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
      platforms = [ "x86_64-linux" ];
    };
  };

in {
  x86_64  = combined;
  aarch64 = combined;
}
