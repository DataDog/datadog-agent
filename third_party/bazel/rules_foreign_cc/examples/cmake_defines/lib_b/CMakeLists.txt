project(cmake_defines)

add_library(lib_b lib_b.cpp)
install(TARGETS lib_b ARCHIVE DESTINATION lib)
