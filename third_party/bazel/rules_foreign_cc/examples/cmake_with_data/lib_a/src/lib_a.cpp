#include "lib_a.h"

#include <cassert>
#include <fstream>
#include <iostream>
#include <stdexcept>

std::string hello_data(std::string path) {
    // Open the file
    std::ifstream data_file(path.c_str());
    if (!data_file.good()) {
        throw std::runtime_error("Could not open: " + path);
    }

    // Read it's contents to a string
    std::string data_str((std::istreambuf_iterator<char>(data_file)),
                         (std::istreambuf_iterator<char>()));

    // Close the file
    data_file.close();

    return data_str;
}
