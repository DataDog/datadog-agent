project('liba', 'cpp')

builddir = 'rules_foreign_cc_build'

cxx = meson.get_compiler('cpp')

liba = static_library(
  'a',
  'liba.cpp',
  cpp_args: cxx.get_supported_arguments(),
  include_directories: include_directories('.'),
  install: true,
  install_dir: join_paths(get_option('prefix'), 'lib'),
)

install_headers('liba.h', subdir: 'include')
