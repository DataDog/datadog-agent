#include "compiler.h"

extern "C" {

struct bpf_compiler {
    bpf_compiler(const char* cpp_compiler_name): cpp_compiler(cpp_compiler_name) {}

    ClangCompiler cpp_compiler;
};

#include "shim.h"

bpf_compiler *new_bpf_compiler(void)
{
    return new bpf_compiler("clang");
}

int bpf_compile_to_object_file(bpf_compiler *compiler, const char *input, const char *output_file, const char **cflagsv, char verbose, char in_memory)
{
    if (!compiler || !input || !output_file) {
        return -1;
    }

    auto& cppCompiler = compiler->cpp_compiler;
    std::vector<const char*> cflags;
    if (cflagsv) {
        while(*cflagsv) {
            cflags.push_back(*cflagsv);
            cflagsv++;
        }
    }
    auto module = cppCompiler.compileToBytecode(input, NULL, cflags, bool(verbose), bool(in_memory));
    if (!module) {
        return -1;
    }
    return cppCompiler.bytecodeToObjectFile(*module, output_file);
}

const char * bpf_compiler_get_errors(bpf_compiler *compiler)
{
    if (!compiler) {
        return NULL;
    }
    return compiler->cpp_compiler.getErrors().c_str();
}

void delete_bpf_compiler(bpf_compiler *compiler)
{
    if (!compiler) {
        return;
    }
    delete compiler;
}

}
