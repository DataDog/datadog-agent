# common definitions for rtloader build

COMMON_DEFINES = [
    "_GLIBCXX_USE_CXX11_ABI=0",
]

COMMON_CXXOPTS = [
    # This was ported from the equivalent original CMake `set(CMAKE_CXX_STANDARD 11)` et al.
    "-std=c++11",
]
