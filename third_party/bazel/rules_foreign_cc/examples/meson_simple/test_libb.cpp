#include <iostream>
#include <stdexcept>
#include <string>

#include "include/liba.h"

int main(int argc, char* argv[]) {
    std::string result = hello_liba();
    if (result != "Hello from LIBA!") {
        throw std::runtime_error("Wrong result: " + result);
    }
    std::cout << "Everything's fine!";
}
