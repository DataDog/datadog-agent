# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-2019 Datadog, Inc.

name 'libc++'
default_version 'llvmorg-9.0.1'

source git: 'https://github.com/llvm/llvm-project.git'

relative_path 'llvm-project'

build do
  command "cmake llvm -DCMAKE_INSTALL_PREFIX=#{install_dir}/embedded -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ -DLLVM_ENABLE_PROJECTS=\"libcxx;libcxxabi\" -DLLVM_ENABLE_LIBCXX=true"
  make "-j #{workers}"
  make 'install-cxx install-cxxabi'
end
