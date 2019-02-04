# Get all project files
file(GLOB_RECURSE
     ALL_SOURCE_FILES
     *.[ch]pp *.[ch]xx *.cc *.h
    )

# Adding clang-format target if executable is found
find_program(
    CLANG_FORMAT
    NAMES clang-format-8 clang-format
    PATHS "/usr/local/bin"
    )
if(CLANG_FORMAT)
  add_custom_target(
    clang-format
    COMMAND ${CLANG_FORMAT}
    -i
    -style=file
    ${ALL_SOURCE_FILES}
    )
endif()
