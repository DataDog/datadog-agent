cmake_minimum_required(VERSION 3.15)
find_package (Python3 COMPONENTS Interpreter Development)

if(Python3_VERSION_MINOR LESS "7")
  message(
    FATAL_ERROR
    "Python3 version found is too old: found ${Python3_EXECUTABLE} (version \"${Python3_VERSION}\"), minimum required version is 3.7"
  )
endif()

project(datadog-agent-three VERSION 0.1.0 DESCRIPTION "CPython backend for the Datadog Agent")

if(WIN32)
  if(MSVC)
    # explicitly set the compiler flags to use the static C runtime (/MT(d) instead of the DLL
    # c runtime (/MD(d) so that we don't have to worry about redistributing the CRT).

    foreach(flag_var
            CMAKE_CXX_FLAGS CMAKE_CXX_FLAGS_DEBUG CMAKE_CXX_FLAGS_RELEASE
            CMAKE_CXX_FLAGS_MINSIZEREL CMAKE_CXX_FLAGS_RELWITHDEBINFO)
    if(${flag_var} MATCHES "/MD")
        string(REGEX REPLACE "/MD" "/MT" ${flag_var} "${${flag_var}}")
    endif(${flag_var} MATCHES "/MD")
    endforeach(flag_var)
  else()
     #assume that it's the gnu generator
     set(CMAKE_C_FLAGS "-D_hypot=hypot")
     set(CMAKE_CXX_FLAGS "-D_hypot=hypot")
  endif()
  string(REPLACE "\\" "\\\\" Python3_STDLIB ${Python3_STDLIB})
endif()

include(GNUInstallDirs)

configure_file(constants.h.in constants.h)
include_directories(${CMAKE_CURRENT_BINARY_DIR})
add_library(datadog-agent-three SHARED
    three.cpp
    three_mem.cpp
    ../common/cgo_free.c
    ../common/stringutils.c
    ../common/log.c
    ../common/builtins/aggregator.c
    ../common/builtins/datadog_agent.c
    ../common/builtins/util.c
    ../common/builtins/_util.c
    ../common/builtins/tagger.c
    ../common/builtins/kubeutil.c
    ../common/builtins/containers.c
)

if(WIN32)
  set_target_properties(datadog-agent-three PROPERTIES LINK_FLAGS -static)
elseif(APPLE)
  set_target_properties(datadog-agent-three PROPERTIES INSTALL_RPATH_USE_LINK_PATH TRUE)
endif()

add_compile_definitions(DATADOG_AGENT_THREE)
target_include_directories(datadog-agent-three PRIVATE .)
target_include_directories(datadog-agent-three PUBLIC
    ${CMAKE_SOURCE_DIR}/include
    ${CMAKE_SOURCE_DIR}/common
    ${CMAKE_SOURCE_DIR}/common/builtins
    ${Python3_INCLUDE_DIRS}
)
target_link_libraries(datadog-agent-three ${Python3_LIBRARIES} datadog-agent-rtloader)

if(WIN32)
  install(TARGETS datadog-agent-three
      RUNTIME DESTINATION ${CMAKE_INSTALL_LIBDIR}
  )
else()
  target_compile_options(datadog-agent-three PRIVATE "-Wno-deprecated-register")
  install(TARGETS datadog-agent-three
      LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
  )
endif()
