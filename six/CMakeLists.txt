cmake_minimum_required(VERSION 3.9)

## Add Build Targets
add_subdirectory(two)
add_subdirectory(three)
add_subdirectory(six)
add_subdirectory(demo)

## Dev tools
include(cmake/clang-format.cmake)
