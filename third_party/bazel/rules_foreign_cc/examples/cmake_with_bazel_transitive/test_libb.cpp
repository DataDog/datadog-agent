#include <iostream>
#include <stdexcept>
#include <string>

#include "libb.h"

int main(int argc, char* argv[]) {
    std::string result = hello_libb();
    if (result != "Hello from LIBA! Hello from LIBB!") {
        throw std::runtime_error("Wrong result: " + result);
    }
    std::cout << "Everything's fine!";
}
