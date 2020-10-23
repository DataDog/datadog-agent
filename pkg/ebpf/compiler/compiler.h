#ifndef __COMPILER_H
#define __COMPILER_H

#include <clang/Driver/Compilation.h>
#include <clang/Frontend/CompilerInvocation.h>
#include <clang/Frontend/FrontendDiagnostic.h>
#include <clang/Frontend/TextDiagnosticPrinter.h>
#include <clang/Driver/Driver.h>
#include <clang/Basic/FileManager.h>

#include <llvm/IR/LLVMContext.h>
#include <llvm/IR/Module.h>

#include <llvm/Target/TargetMachine.h>
#include <llvm/Support/TargetRegistry.h>

class ClangCompiler {
protected:

    static bool llvmInitialized;

	llvm::IntrusiveRefCntPtr<clang::DiagnosticOptions> diagOpts;
	llvm::IntrusiveRefCntPtr<clang::DiagnosticIDs> diagID;
    std::unique_ptr<clang::TextDiagnosticPrinter> textDiagnosticPrinter;
 	std::unique_ptr<clang::DiagnosticsEngine> diagnosticsEngine;
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
        const char *inputFile,
        const char *outputFile = NULL,
        const std::vector<const char*> &cflags = std::vector<const char*>(),
        bool verbose = false);
    int bytecodeToObjectFile(llvm::Module *module, const char *outputFile);
    const std::string& getErrors();
    ~ClangCompiler();

private:

    static llvm::StringRef getDataLayout();
    static llvm::StringRef getArch();
    std::unique_ptr<clang::CompilerInvocation> buildCompilation(
        const char *inputFile,
        const char *outputFile,
        const std::vector<const char*> &cflags,
        bool verbose);
};

#endif
