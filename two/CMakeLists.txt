cmake_minimum_required(VERSION 3.9)
find_package (Python2 COMPONENTS Interpreter Development)

project(datadog-agent-two VERSION 0.1.0 DESCRIPTION "CPython backend for the Datadog Agent")

if(WIN32)
foreach(flag_var
        CMAKE_CXX_FLAGS CMAKE_CXX_FLAGS_DEBUG CMAKE_CXX_FLAGS_RELEASE
        CMAKE_CXX_FLAGS_MINSIZEREL CMAKE_CXX_FLAGS_RELWITHDEBINFO)
   if(${flag_var} MATCHES "/MD")
      string(REGEX REPLACE "/MD" "/MT" ${flag_var} "${${flag_var}}")
   endif(${flag_var} MATCHES "/MD")
endforeach(flag_var)
endif()


include(GNUInstallDirs)
configure_file(constants.h.in constants.h)
include_directories(${CMAKE_CURRENT_BINARY_DIR})
add_library(datadog-agent-two SHARED
    two.cpp
)
target_include_directories(datadog-agent-two PRIVATE .)
target_include_directories(datadog-agent-two PUBLIC
    ${CMAKE_SOURCE_DIR}/include
    ${Python2_INCLUDE_DIRS}
)
target_link_libraries(datadog-agent-two ${Python2_LIBRARIES} datadog-agent-six)

if(NOT WIN32)
    target_compile_options(datadog-agent-two PRIVATE "-Wno-deprecated-register")
endif()

if(WIN32)
  install(TARGETS datadog-agent-two
      RUNTIME DESTINATION ${CMAKE_INSTALL_LIBDIR}
  )
else()
  install(TARGETS datadog-agent-two
      LIBRARY DESTINATION ${CMAKE_INSTALL_LIBDIR}
  )
endif()
