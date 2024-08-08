cmake_minimum_required(VERSION 3.12)

## Project name
project(RtLoader)

## Require C++11 with no extensions
set(CMAKE_CXX_STANDARD 11)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

if(WIN32)
    set(CMAKE_RUNTIME_OUTPUT_DIRECTORY ${CMAKE_SOURCE_DIR}/bin )
    SET(CMAKE_CXX_FLAGS  "${CMAKE_CXX_FLAGS} -mwin32")
elseif(LINUX)
  set(CMAKE_SHARED_LINKER_FLAGS "${CMAKE_SHARED_LINKER_FLAGS} -static-libstdc++")
endif()

if(ARCH_I386)
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -m32")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -m32")
endif()

## Config options
option(DISABLE_PYTHON2 "Do not build Python2 support")
option(DISABLE_PYTHON3 "Do not build Python3 support")
option(BUILD_DEMO "Build the demo app" ON)

## Add Build Targets
if (NOT DISABLE_PYTHON2)
    add_subdirectory(two)
endif()
if (NOT DISABLE_PYTHON3)
    add_subdirectory(three)
endif()
add_subdirectory(rtloader)
add_subdirectory(test)
if (BUILD_DEMO)
    add_subdirectory(demo)
endif()

## Dev tools
include(cmake/clang-format.cmake)
