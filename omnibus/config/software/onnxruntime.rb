# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name 'onnxruntime'
default_version '1.23.2'

license "MIT"
license_file "LICENSE"

# Platform and architecture mapping for ONNX Runtime releases
if linux_target?
  if arm7l_target?
    platform_arch = "aarch64"
    platform_os = "linux"
  elsif arm64_target?
    platform_arch = "aarch64"
    platform_os = "linux"
  else
    platform_arch = "x64"
    platform_os = "linux"
  end
elsif osx_target?
  if arm_target?
    platform_arch = "arm64"
    platform_os = "osx"
  else
    platform_arch = "x64"
    platform_os = "osx"
  end
elsif windows_target?
  platform_arch = windows_arch_i386? ? "x86" : "x64"
  platform_os = "win"
else
  raise "Unsupported platform for onnxruntime"
end

# SHA256 checksums for version 1.23.2
# TODO: Add actual SHA256 values from GitHub releases for each platform
# You can get these by downloading the release files and running: shasum -a 256 <file>
onnxruntime_hashes = {
  "linux-x64" => "1fa4dcaef22f6f7d5cd81b28c2800414350c10116f5fdd46a2160082551c5f9b",    # TODO: Add SHA256 for Linux x64
  "linux-aarch64" => "7c63c73560ed76b1fac6cff8204ffe34fe180e70d6582b5332ec094810241e5c",  # TODO: Add SHA256 for Linux ARM64
  "osx-x64" => "d10359e16347b57d9959f7e80a225a5b4a66ed7d7e007274a15cae86836485a6",      # TODO: Add SHA256 for macOS x64
  "osx-arm64" => "b4d513ab2b26f088c66891dbbc1408166708773d7cc4163de7bdca0e9bbb7856",    # TODO: Add SHA256 for macOS ARM64
  "win-x64" => "",      # TODO: Add SHA256 for Windows x64
  "win-x86" => "",      # TODO: Add SHA256 for Windows x86
}

package_target = "#{platform_os}-#{platform_arch}"
onnxruntime_hash = onnxruntime_hashes[package_target]

version("1.23.2") do
  # Note: SHA256 verification will be skipped if hash is empty
  # In production builds, ensure all hashes are filled in
  source sha256: onnxruntime_hash if onnxruntime_hash && !onnxruntime_hash.empty?
end

ship_source_offer true

# ONNX Runtime release URL pattern: onnxruntime-{os}-{arch}-{version}.tgz
# For Windows: onnxruntime-{os}-{arch}-{version}.zip
source_url = if windows_target?
  "https://github.com/microsoft/onnxruntime/releases/download/v#{version}/onnxruntime-#{platform_os}-#{platform_arch}-#{version}.zip"
else
  "https://github.com/microsoft/onnxruntime/releases/download/v#{version}/onnxruntime-#{platform_os}-#{platform_arch}-#{version}.tgz"
end

source url: source_url,
        extract: :seven_zip

relative_path "onnxruntime-#{platform_os}-#{platform_arch}-#{version}"

build do
  # Copy libraries to embedded/lib (or embedded/bin for Windows DLLs)
  if linux_target?
    mkdir "#{install_dir}/embedded/lib"
    copy "lib/libonnxruntime.so*", "#{install_dir}/embedded/lib/"
  elsif osx_target?
    mkdir "#{install_dir}/embedded/lib"
    copy "lib/libonnxruntime.dylib*", "#{install_dir}/embedded/lib/"
  elsif windows_target?
    mkdir "#{install_dir}/embedded/bin"
    mkdir "#{install_dir}/embedded/lib"
    copy "lib/onnxruntime.dll", "#{install_dir}/embedded/bin/"
    copy "lib/onnxruntime.lib", "#{install_dir}/embedded/lib/"
  end

  # Copy headers to embedded/include/onnxruntime
  mkdir "#{install_dir}/embedded/include/onnxruntime"
  copy "include/onnxruntime/*.h", "#{install_dir}/embedded/include/onnxruntime/"
end

