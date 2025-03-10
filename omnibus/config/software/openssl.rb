#
# Copyright 2012-2016 Chef Software, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

name "openssl"

license "OpenSSL"
license_file "LICENSE"
skip_transitive_dependency_licensing true

dependency "zlib"
dependency "cacerts"
dependency "makedepend" unless aix? || windows?

default_version "1.1.1u"

# OpenSSL source ships with broken symlinks which windows doesn't allow.
# Skip error checking.

# Note: Since April 2020, only the most recent version of openssl is
# available at https://www.openssl.org/source/.
# Older versions of openssl are now there: https://www.openssl.org/source/old/
# This software definition may break every time openssl is updated.
# To build with an older version of openssl, you'll need to update the source url to
# https://www.openssl.org/source/old/<bugfix_version>/openssl-<full_version>.tar.gz
source url: "https://www.openssl.org/source/openssl-#{version}.tar.gz", extract: :lax_tar

version("1.1.1u") { source sha256: "e2f8d84b523eecd06c7be7626830370300fbcc15386bf5142d72758f6963ebc6" }
version("1.1.1t") { source sha256: "8dee9b24bdb1dcbf0c3d1e9b02fb8f6bf22165e807f45adeb7c9677536859d3b" }
version("1.1.1q") { source sha256: "d7939ce614029cdff0b6c20f0e2e5703158a489a72b2507b8bd51bf8c8fd10ca" }
version("1.1.1p") { source sha256: "bf61b62aaa66c7c7639942a94de4c9ae8280c08f17d4eac2e44644d9fc8ace6f" }
version("1.1.1o") { source sha256: "9384a2b0570dd80358841464677115df785edb941c71211f75076d72fe6b438f" }
version("1.1.1n") { source sha256: "40dceb51a4f6a5275bde0e6bf20ef4b91bfc32ed57c0552e2e8e15463372b17a" }
version("1.1.1l") { source sha256: "0b7a3e5e59c34827fe0c3a74b7ec8baef302b98fa80088d7f9153aa16fa76bd1" }
version("1.1.1k") { source sha256: "892a0875b9872acd04a9fde79b1f943075d5ea162415de3047c327df33fbaee5" }
version("1.1.1i") { source sha256: "e8be6a35fe41d10603c3cc635e93289ed00bf34b79671a3a4de64fcee00d5242" }
version("1.1.1h") { source sha256: "5c9ca8774bd7b03e5784f26ae9e9e6d749c9da2438545077e6b3d755a06595d9" }
version("1.1.1f") { source sha256: "186c6bfe6ecfba7a5b48c47f8a1673d0f3b0e5ba2e25602dd23b629975da3f35" }
version("1.1.1d") { source sha256: "1e3a91bc1f9dfce01af26026f856e064eab4c8ee0a8f457b5ae30b40b8b711f2" }
version("1.0.2t") { source sha256: "14cb464efe7ac6b54799b34456bd69558a749a4931ecfd9cf9f71d7881cac7bc" }
version("1.0.2o") { source sha256: "ec3f5c9714ba0fd45cb4e087301eb1336c317e0d20b575a125050470e8089e4d" }
version("1.0.2k") { source sha256: "6b3977c61f2aedf0f96367dcfb5c6e578cf37e7b8d913b4ecb6643c3cb88d8c0" }
version("1.0.2j") { source sha256: "e7aff292be21c259c6af26469c7a9b3ba26e9abaaffd325e3dccc9785256c431" }
version("1.0.2i") { source sha256: "9287487d11c9545b6efb287cdb70535d4e9b284dd10d51441d9b9963d000de6f" }
version("1.0.2h") { source sha256: "1d4007e53aad94a5b2002fe045ee7bb0b3d98f1a47f8b2bc851dcd1c74332919" }
version("1.0.1u") { source sha256: "4312b4ca1215b6f2c97007503d80db80d5157f76f8f7d3febbe6b4c56ff26739" }

relative_path "openssl-#{version}"

build do

  env = with_standard_compiler_flags(with_embedded_path)
  if aix?
    env["M4"] = "/opt/freeware/bin/m4"
  elsif freebsd?
    # Should this just be in standard_compiler_flags?
    env["LDFLAGS"] += " -Wl,-rpath,#{install_dir}/embedded/lib"
  elsif windows?
    # XXX: OpenSSL explicitly sets -march=i486 and expects that to be honored.
    # It has OPENSSL_IA32_SSE2 controlling whether it emits optimized SSE2 code
    # and the 32-bit calling convention involving XMM registers is...  vague.
    # Do not enable SSE2 generally because the hand optimized assembly will
    # overwrite registers that mingw expects to get preserved.
    env["CFLAGS"] = "-I#{install_dir}/embedded/include"
    env["CPPFLAGS"] = env["CFLAGS"]
    env["CXXFLAGS"] = env["CFLAGS"]
  end

  configure_args = [
    "--prefix=#{install_dir}/embedded",
    "--with-zlib-lib=#{install_dir}/embedded/lib",
    "--with-zlib-include=#{install_dir}/embedded/include",
    "no-idea",
    "no-mdc2",
    "no-rc5",
    "shared",
    "no-ssl3",
  ]

  if windows?
    configure_args << "zlib-dynamic"
  else
    configure_args << "zlib"
  end

  configure_cmd =
    if aix?
      "perl ./Configure aix64-cc"
    elsif mac_os_x?
      "./Configure darwin64-x86_64-cc"
    elsif smartos?
      "/bin/bash ./Configure solaris64-x86_64-gcc -static-libgcc"
    elsif omnios?
      "/bin/bash ./Configure solaris-x86-gcc"
    elsif solaris_10?
      # This should not require a /bin/sh, but without it we get
      # Errno::ENOEXEC: Exec format error
      platform = sparc? ? "solaris-sparcv9-gcc" : "solaris-x86-gcc"
      "/bin/sh ./Configure #{platform} -static-libgcc"
    elsif solaris_11?
      platform = sparc? ? "solaris64-sparcv9-gcc" : "solaris64-x86_64-gcc"
      "/bin/bash ./Configure #{platform} -static-libgcc"
    elsif windows?
      platform = windows_arch_i386? ? "mingw" : "mingw64"
      "perl.exe ./Configure #{platform}"
    else
      prefix =
        if linux? && ppc64?
          "./Configure linux-ppc64"
        elsif linux? && s390x?
          # With gcc > 4.3 on s390x there is an error building
          # with inline asm enabled
          "./Configure linux64-s390x -DOPENSSL_NO_INLINE_ASM"
        else
          "./config"
        end
      "#{prefix} disable-gost"
    end

  # on 1.0 we have to path before running config
  if version.start_with? "1.0."
    if aix?
      # This enables omnibus to use 'makedepend'
      # from fileset 'X11.adt.imake' (AIX install media)
      env["PATH"] = "/usr/lpp/X11/bin:#{ENV["PATH"]}"

      patch_env = env.dup
      patch_env["PATH"] = "/opt/freeware/bin:#{env["PATH"]}"
      patch source: "openssl-1.0.1f-do-not-build-docs.patch", env: patch_env
    else
      patch source: "openssl-1.0.1f-do-not-build-docs.patch", env: env
    end

    if windows?
      # Patch Makefile.org to update the compiler flags/options table for mingw.
      patch source: "openssl-1.0.1q-fix-compiler-flags-table-for-msys.patch", env: env
    end
  end

  # Out of abundance of caution, we put the feature flags first and then
  # the crazy platform specific compiler flags at the end.
  configure_args << env["CFLAGS"] << env["LDFLAGS"]

  configure_command = configure_args.unshift(configure_cmd).join(" ")

  command configure_command, env: env, in_msys_bash: true

  # on 1.1 we have to path after running config
  if version.start_with? "1.1."
    if aix?
      # This enables omnibus to use 'makedepend'
      # from fileset 'X11.adt.imake' (AIX install media)
      env["PATH"] = "/usr/lpp/X11/bin:#{ENV["PATH"]}"

      patch_env = env.dup
      patch_env["PATH"] = "/opt/freeware/bin:#{env["PATH"]}"
      patch source: "openssl-1.1.1d-do-not-build-docs.patch", env: patch_env
    else
      patch source: "openssl-1.1.1d-do-not-build-docs.patch", env: env
    end
  end

  command "make depend", env: env
  command "make -j #{workers}", env: env
  if aix?
    # We have to sudo this because you can't actually run slibclean without being root.
    # Something in openssl changed in the build process so now it loads the libcrypto
    # and libssl libraries into AIX's shared library space during the first part of the
    # compile. This means we need to clear the space since it's not being used and we
    # can't install the library that is already in use. Ideally we would patch openssl
    # to make this not be an issue.
    # Bug Ref: http://rt.openssl.org/Ticket/Display.html?id=2986&user=guest&pass=guest
    command "sudo /usr/sbin/slibclean", env: env
  end
  command "make install", env: env

  delete "#{install_dir}/embedded/bin/c_rehash"
  unless windows?
    # Remove openssl static libraries here as we can't disable those at build time
    delete "#{install_dir}/embedded/lib/libcrypto.a"
    delete "#{install_dir}/embedded/lib/libssl.a"
  end
end
