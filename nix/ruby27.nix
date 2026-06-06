{ pkgs }:

# Ruby 2.7.8 built against the nixpkgs-unstable stdenv.
#
# We deliberately do NOT use the nixpkgs-ruby27 (nixos-23.11) package for this:
# that snapshot's Darwin bootstrap chain requires building LLVM 16 from source,
# which fails on macOS 26 (Darwin 25.x / Tahoe) because LLVM 16's TargetParser
# unit tests compare the runtime macOS version to the SDK version baked into the
# binary and abort when they don't match.  Building against the nixpkgs-unstable
# stdenv (LLVM 18+) sidesteps this entirely.
#
# Bundler 2.4.22 is installed into the Ruby gem directory during the build using
# a pre-fetched .gem file (no network access required in the Nix sandbox).
# CI uses bundler 2.4.20; 2.4.22 is API-compatible.

let
  bundlerGem = pkgs.fetchurl {
    url  = "https://rubygems.org/downloads/bundler-2.4.22.gem";
    hash = "sha256-dHulCw5n3yXL07SPlYMad6TVOlgdVfBjly/LFG0ULF8=";
  };
in

pkgs.stdenv.mkDerivation rec {
  pname   = "ruby";
  version = "2.7.8";

  src = pkgs.fetchurl {
    url  = "https://cache.ruby-lang.org/pub/ruby/2.7/ruby-2.7.8.tar.gz";
    hash = "sha256-wtq2PLyPKgVSYQitQZ76Y6Z+1AdNu8+fwrHKZky0W6A=";
  };

  # Ruby 2.7's bundled ext/openssl/extconf.rb hard-rejects OpenSSL >= 3.0.0:
  #   "OpenSSL >= 1.0.1, < 3.0.0 or LibreSSL >= 2.5.0 is required"
  # Use openssl_1_1 (1.1.1w) even though it is EOL; the binary is only used
  # for running omnibus/bundler on the developer machine, never shipped.
  openssl = pkgs.openssl_1_1;

  nativeBuildInputs = [ pkgs.pkg-config ];

  buildInputs = [
    openssl openssl.dev
    pkgs.readline pkgs.readline.dev
    pkgs.zlib pkgs.zlib.dev
    pkgs.libffi pkgs.libffi.dev
    pkgs.libyaml
    pkgs.gdbm
    pkgs.ncurses
  ];

  configureFlags = [
    "--enable-shared"
    "--disable-install-doc"
    "--with-openssl-dir=${openssl.dev}"
  ] ++ pkgs.lib.optionals pkgs.stdenv.isDarwin [
    # dtrace probes require the macOS SDK dtrace; skip for local dev builds.
    "--disable-dtrace"
  ];

  enableParallelBuilding = true;

  postInstall = ''
    # Install bundler 2.4.22 from the pre-fetched gem (no network in sandbox).
    HOME=$TMPDIR $out/bin/gem install --local --no-document ${bundlerGem}
  '';

  meta = {
    description = "Ruby 2.7.8 with bundler 2.4.22, built for the omnibus dev shell";
    platforms   = pkgs.lib.platforms.all;
  };
}
