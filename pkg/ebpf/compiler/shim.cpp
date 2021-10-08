#include "compiler.h"

extern "C" {

struct bpf_compiler {
    ClangCompiler *cpp_compiler;
};

#include "shim.h"

bpf_compiler *new_bpf_compiler(void)
{
    auto cpp_compiler = new ClangCompiler("clang");
    auto c_compiler = new bpf_compiler();
    c_compiler->cpp_compiler = cpp_compiler;
    return c_compiler;
}

int bpf_compile_to_object_file(bpf_compiler *compiler, const char *input, const char *output_file, const char **cflagsv, char verbose, char in_memory)
{
    if (!compiler || !input || !output_file) {
        return -1;
    }

    auto cppCompiler = static_cast<ClangCompiler*>(compiler->cpp_compiler);
    std::vector<const char*> cflags;
    if (cflagsv) {
        while(*cflagsv) {
            cflags.push_back(*cflagsv);
            cflagsv++;
        }
    }
    auto module = cppCompiler->compileToBytecode(input, NULL, cflags, bool(verbose), bool(in_memory));
    if (!module) {
        return -1;
    }
    return cppCompiler->bytecodeToObjectFile(*module, output_file);
}

const char * bpf_compiler_get_errors(bpf_compiler *compiler)
{
    if (!compiler) {
        return NULL;
    }
    auto cppCompiler = static_cast<ClangCompiler*>(compiler->cpp_compiler);
    return cppCompiler->getErrors().c_str();
}

void delete_bpf_compiler(bpf_compiler *compiler)
{
    if (!compiler) {
        return;
    }
    delete compiler->cpp_compiler;
    delete compiler;
}

}
