cmake_minimum_required(VERSION 3.19)

project("RTLoader Demo")

option(USE_AGENT_RTLOADER_LIB "Use RTLoader implementation from the agent build" OFF)

## Use compiler debugging flags per default
set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} ${CMAKE_C_FLAGS_DEBUG}")

add_executable(demo main.c)

## Add our paths for includes. Note that these may be different depending on
## where you have rtloader built and/or checked out
include_directories("${CMAKE_SOURCE_DIR}/../include" "${CMAKE_SOURCE_DIR}/../common")


if(WIN32)
  set_target_properties(demo PROPERTIES LINK_FLAGS -static)
  target_link_libraries(demo PUBLIC datadog-agent-rtloader)
else()
  if (APPLE)
    set(LIB_SUFFIX "dylib")
  else()
    set(LIB_SUFFIX "so")
  endif()


  if (USE_AGENT_RTLOADER_LIB)
    set(RTLOADER_LIB_PREFIX "${CMAKE_SOURCE_DIR}/../build/rtloader/")
  else()
    set(RTLOADER_LIB_PREFIX "")
  endif()

  set(RTLOADER_LIB "${RTLOADER_LIB_PREFIX}libdatadog-agent-rtloader.${LIB_SUFFIX}")
  target_link_libraries(demo PUBLIC "${RTLOADER_LIB}" dl)
endif()
