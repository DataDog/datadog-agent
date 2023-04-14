# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/cmake.rb'

name 'openscap'
default_version '1.3.7'

license "LGPL-3.0-or-later"
license_file "COPYING"

version("1.3.7") { source sha256: "a74f5bfb420b748916d2f88941bb6e04cad4c67a4cafc78c96409cc15c54d1d3" }

ship_source_offer true

source url: "https://github.com/OpenSCAP/openscap/releases/download/#{version}/openscap-#{version}.tar.gz"

dependency 'apt'
dependency 'attr'
dependency 'bzip2'
dependency 'curl'
dependency 'libacl'
dependency 'libgcrypt'
dependency 'libselinux'
dependency 'libsepol'
dependency 'libxslt'
dependency 'libyaml'
dependency 'pcre'
dependency 'popt'
dependency 'rpm'
dependency 'util-linux'
dependency 'xmlsec'

relative_path "openscap-#{version}"

build do
  env = with_standard_compiler_flags(with_embedded_path)

  # Fixes since release 1.3.7
  patch source: "0006-Use-correct-format-specifier.patch", env: env
  patch source: "0007-Fix-leaked-variable.patch", env: env
  patch source: "0008-Fix-a-leaked-variable.patch", env: env
  patch source: "0009-Fix-Wint-conversion-error-building-with-clang.patch", env: env
  patch source: "0010-Remove-reference-to-PROC_CHECK.patch", env: env
  patch source: "0015-Fix-leak-of-session-skip_rules.patch", env: env
  patch source: "0016-Fix-leak-of-dpkginfo_reply_t-fields.patch", env: env

  patch source: "get_results_from_session.patch", env: env # add a function to retrieve results from session
  patch source: "session_result_free.patch", env: env # add a function to free results from session
  patch source: "source_free_xml.patch", env: env # free XML DOM after loading session
  patch source: "010_perlpm_install_fix.patch", env: env # fix build of perl bindings
  patch source: "dpkginfo-cacheconfig.patch", env: env # work around incomplete pkgcache path
  patch source: "oval_component_evaluate_CONCAT_leak.patch", env: env # fix memory leak
  patch source: "dpkginfo-init.patch", env: env # fix memory leak of pkgcache in dpkginfo probe
  patch source: "shadow-chroot.patch", env: env # handle shadow probe in offline mode
  patch source: "fsdev-ignore-host.patch", env: env # ignore /host directory in fsdev probe

  patch source: "oscap-io.patch", env: env # add new oscap-io tool

  env["CC"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/gcc"
  env["CXX"] = "/opt/gcc-#{ENV['GCC_VERSION']}/bin/g++"
  env["CXXFLAGS"] += " -static-libstdc++ -std=c++11 -DDPKG_DATADIR=/usr/share/dpkg"

  cmake_build_dir = "#{project_dir}/build"
  cmake_options = [
    "-DENABLE_PERL=OFF",
    "-DENABLE_PYTHON3=OFF",
    "-DACL_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DACL_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libacl.so",
    "-DAPTPKG_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DAPTPKG_LIBRARIES:FILEPATH=#{install_dir}/embedded/lib/libapt-pkg.so",
    "-DBLKID_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DBLKID_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libblkid.so",
    "-DBZIP2_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DBZIP2_LIBRARY_RELEASE:FILEPATH=#{install_dir}/embedded/lib/libbz2.so",
    "-DCURL_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DCURL_LIBRARY_RELEASE:FILEPATH=#{install_dir}/embedded/lib/libcurl.so",
    "-DGCRYPT_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DGCRYPT_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libgcrypt.so",
    "-DLIBXML2_INCLUDE_DIR:PATH=#{install_dir}/embedded/include/libxml2",
    "-DLIBXML2_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libxml2.so",
    "-DLIBXSLT_EXSLT_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DLIBXSLT_EXSLT_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libexslt.so",
    "-DLIBXSLT_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DLIBXSLT_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libxslt.so",
    "-DLIBYAML_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DLIBYAML_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libyaml.so",
    "-DOPENSSL_CRYPTO_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libcrypto.so",
    "-DOPENSSL_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DOPENSSL_SSL_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libssl.so",
    "-DPCRE_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DPCRE_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libpcre.so",
    "-DPOPT_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DPOPT_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libpopt.so",
    "-DPYTHON_INCLUDE_DIR:PATH=#{install_dir}/embedded/include/python3.8",
    "-DPYTHON_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libpython3.8.so",
    "-DRPM_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DRPMIO_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/librpmio.so",
    "-DRPM_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/librpm.so",
    "-DSELINUX_INCLUDE_DIR:PATH=#{install_dir}/embedded/include",
    "-DSELINUX_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libselinux.so",
    "-DXMLSEC_INCLUDE_DIR:PATH=#{install_dir}/embedded/include/xmlsec1",
    "-DXMLSEC_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libxmlsec1.so",
    "-DXMLSEC_OPENSSL_LIBRARY:FILEPATH=#{install_dir}/embedded/lib/libxmlsec1-openssl.so",
  ]
  cmake(*cmake_options, env: env, cwd: cmake_build_dir, prefix: "#{install_dir}/embedded")

  # Remove OpenSCAP XML schemas, since they are not useful when XSD validation is disabled.
  command "rm -rf #{install_dir}/embedded/share/openscap/schemas"
end
