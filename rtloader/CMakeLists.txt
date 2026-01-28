cmake_minimum_required(VERSION 3.15)

## Project name
project(RtLoader)

## Require C++11 with no extensions
set(CMAKE_CXX_STANDARD 11)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Favor finding Python by location rather than by version
if (POLICY CMP0094)
    cmake_policy(SET CMP0094 NEW)
endif()

if(WIN32)
    set(CMAKE_RUNTIME_OUTPUT_DIRECTORY ${CMAKE_SOURCE_DIR}/bin )
    SET(CMAKE_CXX_FLAGS  "${CMAKE_CXX_FLAGS} -mwin32")
endif()

if(ARCH_I386)
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -m32")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -m32")
endif()

# CPU baseline optimization - x86-64-v3 / ARMv8.1-A
if(CMAKE_SYSTEM_PROCESSOR MATCHES "x86_64|amd64|AMD64")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -march=x86-64-v3")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -march=x86-64-v3")
elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "aarch64|arm64")
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -march=armv8.1-a")
    set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -march=armv8.1-a")
endif()

## Config options
option(BUILD_DEMO "Build the demo app" ON)

## Add Build Targets
add_subdirectory(three)
add_subdirectory(rtloader)
add_subdirectory(test)
if (BUILD_DEMO)
    add_subdirectory(demo)
endif()

## Dev tools
include(cmake/clang-format.cmake)
