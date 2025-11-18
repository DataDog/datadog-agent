#include <fstream>
#include <iostream>
#include <stdexcept>
#include <string>

void test_opening_file(std::string path) {
    std::ifstream data_file(path);
    if (!data_file.good()) {
        throw std::runtime_error("Could not open file: " + path);
    }

    data_file.close();
}

int main(int argc, char* argv[]) {
    // Make sure the expectd shared library is available
#ifdef _WIN32
    test_opening_file(".\\cmake_with_data\\lib_b\\lib_b.dll");
#else
    // Shared libraries used to have the .so file extension on macOS.
    // See https://github.com/bazelbuild/bazel/pull/14369.
    try {
        test_opening_file("./cmake_with_data/lib_b/liblib_b.so");
    } catch (std::runtime_error& e) {
        test_opening_file("./cmake_with_data/lib_b/liblib_b.dylib");
    }
#endif
    std::cout << "Everything's fine!";
}
