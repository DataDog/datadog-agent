project(cmake_defines)

add_library(lib_a lib_a.cpp)
install(TARGETS lib_a ARCHIVE DESTINATION lib)
