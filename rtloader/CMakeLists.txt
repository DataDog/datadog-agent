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

## Python 3.14+ sub-interpreter support.
## Pass -DENABLE_SUBINTERPRETERS=ON to cmake to enable.
## Defines RTLOADER_HAS_SUBINTERPRETERS for all subdirectories (three, rtloader, test).
option(ENABLE_SUBINTERPRETERS "Enable experimental Python sub-interpreter support (requires Python 3.14+)" OFF)

if(ENABLE_SUBINTERPRETERS)
  find_package(Python3 COMPONENTS Interpreter)
  if(Python3_VERSION VERSION_LESS "3.14")
    message(FATAL_ERROR
      "ENABLE_SUBINTERPRETERS=ON requires Python 3.14+, but found Python ${Python3_VERSION}")
  endif()
  message(STATUS "Sub-interpreter support enabled (Python ${Python3_VERSION})")
  # NOTE: Each subdirectory adds RTLOADER_HAS_SUBINTERPRETERS to its own
  # target via target_compile_definitions. We don't use add_compile_definitions
  # here because it doesn't reliably propagate across project() boundaries
  # in subdirectories.
endif()

## Add Build Targets
add_subdirectory(three)
add_subdirectory(rtloader)
add_subdirectory(test)

## Dev tools
include(cmake/clang-format.cmake)
