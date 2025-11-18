# taken from https://github.com/Akagi201/learning-cmake/tree/master/hello-world-lib
# for test only

cmake_minimum_required(VERSION 2.8.4)

add_library(hello_shared SHARED hello.c hello.h)

IF (WIN32)
  set(LIB_NAME "libhello")
ELSE()
  set(LIB_NAME "hello")
ENDIF()

set_target_properties(hello_shared PROPERTIES
  OUTPUT_NAME ${LIB_NAME}
  )

install(
 TARGETS
 hello_shared
 LIBRARY DESTINATION lib
)

install(FILES hello.h DESTINATION include)
