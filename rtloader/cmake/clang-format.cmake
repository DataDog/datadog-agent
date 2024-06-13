# Get all project files
file(GLOB_RECURSE
     ALL_SOURCE_FILES
     *.[ch]pp *.[ch]xx *.cc *.h
    )
list(FILTER ALL_SOURCE_FILES EXCLUDE REGEX ".CMakeFiles.")

# Adding clang-format target if executable is found
if(WIN32)
    find_program(
        CLANG_FORMAT
        NAMES clang-format-8 clang-format
        PATHS "c:\\devtools\\llvm\\bin" "c:\\Program Files\\LLVM\\bin"
        )
else()
    find_program(
        CLANG_FORMAT
        NAMES clang-format-8 clang-format
        PATHS "/usr/bin"
        )
endif()
if(CLANG_FORMAT)
  add_custom_target(
    clang-format
    COMMAND ${CLANG_FORMAT}
    -i
    -style=file
    ${ALL_SOURCE_FILES}
    )
endif()
