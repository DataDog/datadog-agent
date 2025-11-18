#include <iostream>
#include <stdexcept>
#include <string>

#include "liba.h"

int main(int argc, char* argv[]) {
    std::string result = hello_liba();
    if (result != "Hello from LIBA!") {
        throw std::runtime_error("Wrong result: " + result);
    }
    double math_result = hello_math(0.5);
    if (math_result < 1.047 || math_result > 1.048) {
        throw std::runtime_error("Wrong math_result: " +
                                 std::to_string(math_result));
    }
    std::cout << "Everything's fine!";
}
