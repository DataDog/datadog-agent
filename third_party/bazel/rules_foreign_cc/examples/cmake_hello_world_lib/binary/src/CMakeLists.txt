# taken from https://github.com/Akagi201/learning-cmake/tree/master/hello-world-lib
# for test only

cmake_minimum_required(VERSION 2.8.4)

add_library(hello_lib STATIC hello.c hello.h)

IF (WIN32)
  set(LIB_NAME "libhello")
ELSE()
  set(LIB_NAME "hello")
ENDIF()

set_target_properties(hello_lib PROPERTIES
  OUTPUT_NAME ${LIB_NAME}
  )

add_executable(myapp main.c)
target_link_libraries(myapp hello_lib)
target_include_directories(myapp PUBLIC
  $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
)
set_target_properties(myapp PROPERTIES
  OUTPUT_NAME hello
  )

install(
 TARGETS
 hello_lib
 ARCHIVE DESTINATION lib
)

install(
  TARGETS myapp DESTINATION bin
)

install(FILES hello.h DESTINATION include)
