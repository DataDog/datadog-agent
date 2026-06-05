{ pkgs, ... }:

# CPython 3.12.6 — matches PY3_VERSION in docker-bake.hcl.
# Overrides the nixpkgs python312 package to pin the exact release version.
# nixpkgs already builds Python with --enable-shared on Linux, so
# $out/lib/libpython3.12.so is available for rtloader's dlopen.
#
# Patches are cleared: nixpkgs python312 may be at a different 3.12.x
# patch level and its patches may not apply cleanly against 3.12.6.
pkgs.python312.overrideAttrs (_old: {
  version = "3.12.6";
  src = pkgs.fetchurl {
    url = "https://www.python.org/ftp/python/3.12.6/Python-3.12.6.tgz";
    hash = "sha256-haTBvpBtIOXFpp8kZrANp2nCIdamhKz9OlFNv1vxCmY=";
  };
  patches = [];
})
