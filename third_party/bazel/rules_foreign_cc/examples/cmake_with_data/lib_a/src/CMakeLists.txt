set(LIB_SRC lib_a.cpp lib_a.h)

add_library(lib_a_static STATIC ${LIB_SRC})

set_target_properties(lib_a_static PROPERTIES OUTPUT_NAME "lib_a")
set_target_properties(lib_a_static PROPERTIES ARCHIVE_OUTPUT_NAME "lib_a")
set_target_properties(lib_a_static PROPERTIES PREFIX "")

set(LIBRARY_OUTPUT_PATH ${PROJECT_BINARY_DIR}/lib)

install(TARGETS lib_a_static ARCHIVE DESTINATION lib)
install(FILES lib_a.h DESTINATION include)
