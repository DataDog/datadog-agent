cmake_minimum_required(VERSION 2.8.4)

project(liba)

set(LIBA_SRC ../src/liba.cpp ../src/liba.h)

add_library(liba_static STATIC ${LIBA_SRC})

set_target_properties(liba_static PROPERTIES OUTPUT_NAME "liba")
IF (WIN32)
  set_target_properties(liba_static PROPERTIES ARCHIVE_OUTPUT_NAME "liba")
ELSE()
  set_target_properties(liba_static PROPERTIES ARCHIVE_OUTPUT_NAME "a")
ENDIF()
set_target_properties(liba_static PROPERTIES PUBLIC_HEADER "../src/liba.h")

set(LIBRARY_OUTPUT_PATH ${PROJECT_BINARY_DIR}/lib)

install(FILES ../src/liba.h DESTINATION include)

install(TARGETS liba_static
  EXPORT liba_targets
  ARCHIVE DESTINATION lib
  INCLUDES DESTINATION include
  PUBLIC_HEADER DESTINATION include
  )
target_include_directories(liba_static INTERFACE
    $<INSTALL_INTERFACE:include>
)

install(EXPORT liba_targets
  DESTINATION lib/cmake/liba
  FILE liba-config.cmake)
