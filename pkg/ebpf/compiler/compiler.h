#ifndef __COMPILER_H
#define __COMPILER_H

#include <mutex>
#include <clang/Driver/Compilation.h>
#include <clang/Frontend/CompilerInvocation.h>
#include <clang/Frontend/FrontendDiagnostic.h>
#include <clang/Frontend/TextDiagnosticPrinter.h>
#include <clang/Driver/Driver.h>
#include <clang/Basic/FileManager.h>
#include <clang/Lex/PreprocessorOptions.h>

#include <llvm/IR/LLVMContext.h>
#include <llvm/IR/Module.h>

#include <llvm/Target/TargetMachine.h>
#include <llvm/Support/TargetRegistry.h>

class ClangCompiler {
protected:
    llvm::IntrusiveRefCntPtr<clang::DiagnosticOptions> diagOpts;
    llvm::IntrusiveRefCntPtr<clang::DiagnosticIDs> diagID;
    std::unique_ptr<clang::TextDiagnosticPrinter> textDiagnosticPrinter;
    llvm::IntrusiveRefCntPtr<clang::DiagnosticsEngine> diagnosticsEngine;
    std::unique_ptr<llvm::LLVMContext> llvmContext;
    std::unique_ptr<clang::driver::Driver> theDriver;
    llvm::Triple theTriple;
    std::unique_ptr<llvm::TargetMachine> targetMachine;
    const llvm::Target *theTarget;
    std::vector<const char*> defaultCflags;

    llvm::raw_string_ostream errStream;
    std::string errString;

    std::vector<std::string> includeDirs;

public:

    ClangCompiler(const char *name);
    std::unique_ptr<llvm::Module> compileToBytecode(
        const char *input,
        const char *outputFile = NULL,
        const std::vector<const char*> &cflags = std::vector<const char*>(),
        bool verbose = false,
        bool inMemory = true);
    int bytecodeToObjectFile(llvm::Module &module, const char *outputFile);
    const std::string& getErrors() const;
    ~ClangCompiler();

private:
    static const std::string main_path;
    static std::once_flag llvmInitialized;
    static std::map<std::string, std::unique_ptr<llvm::MemoryBuffer>> remapped_files;
    static llvm::StringRef getDataLayout();
    static llvm::StringRef getArch();
    std::unique_ptr<clang::CompilerInvocation> buildCompilation(
        const char *inputFile,
        const char *outputFile,
        const std::vector<const char*> &cflags,
        bool verbose,
        bool inMemory);
};

#endif
