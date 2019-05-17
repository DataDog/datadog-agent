cmake_minimum_required(VERSION 3.9)
find_package (Python2 COMPONENTS Interpreter Development)

project(datadog-agent-two VERSION 0.1.0 DESCRIPTION "CPython backend for the Datadog Agent")

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
  else() # assume gnuC on windows
    set(CMAKE_C_FLAGS "-D_hypot=hypot -DMS_WIN64")
    set(CMAKE_CXX_FLAGS "-D_hypot=hypot -DMS_WIN64")
    string(REPLACE "\\" "\\\\" Python2_STDLIB ${Python2_STDLIB})
  endif()
endif()

include(GNUInstallDirs)
configure_file(constants.h.in constants.h)
include_directories(${CMAKE_CURRENT_BINARY_DIR})
add_library(datadog-agent-two SHARED
    two.cpp
    ../common/cgo_free.c
    ../common/stringutils.c
    ../common/builtins/aggregator.c
    ../common/builtins/datadog_agent.c
    ../common/builtins/util.c
    ../common/builtins/_util.c
    ../common/builtins/tagger.c
    ../common/builtins/kubeutil.c
    ../common/builtins/containers.c
)
add_compile_definitions(DATADOG_AGENT_TWO)
target_include_directories(datadog-agent-two PRIVATE .)
target_include_directories(datadog-agent-two PUBLIC
    ${CMAKE_SOURCE_DIR}/include
    ${CMAKE_SOURCE_DIR}/common
    ${CMAKE_SOURCE_DIR}/common/builtins
    ${Python2_INCLUDE_DIRS}
)
if(WIN32)
  set_target_properties(datadog-agent-two PROPERTIES LINK_FLAGS -static)
endif()

target_link_libraries(datadog-agent-two ${Python2_LIBRARIES} datadog-agent-six)

if(WIN32)
  install(TARGETS datadog-agent-two
      RUNTIME DESTINATION ${CMAKE_INSTALL_LIBDIR}
  )
else()
  target_compile_options(datadog-agent-two PRIVATE "-Wno-deprecated-register")
  install(TARGETS datadog-agent-two
      LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
  )
endif()
