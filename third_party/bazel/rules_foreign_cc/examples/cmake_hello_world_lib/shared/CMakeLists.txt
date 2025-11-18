# taken from https://github.com/Akagi201/learning-cmake/tree/master/hello-world-lib
# for test only

cmake_minimum_required(VERSION 2.8.4)

set(CMAKE_WINDOWS_EXPORT_ALL_SYMBOLS ON)

project(hellolib LANGUAGES C)

add_subdirectory(src)
