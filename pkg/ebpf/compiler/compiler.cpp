#include "compiler.h"

#include <iostream>
#include <llvm/IR/LegacyPassManager.h>
#include <llvm/Support/TargetSelect.h>
#include <clang/CodeGen/CodeGenAction.h>
#include <clang/Driver/Job.h>
#include <clang/Driver/Tool.h>
#include <clang/Frontend/CompilerInstance.h>

bool ClangCompiler::llvmInitialized = false;

enum Architecture { PPC, PPCLE, S390X, ARM64, X86 };

ClangCompiler::ClangCompiler(const char *name) :
    llvmContext(new llvm::LLVMContext),
    diagOpts(new clang::DiagnosticOptions()),
    errStream(errString),
    textDiagnosticPrinter(new clang::TextDiagnosticPrinter(errStream, diagOpts.get())),
    diagnosticsEngine(new clang::DiagnosticsEngine(diagID, diagOpts, textDiagnosticPrinter.get(), false)),
    defaultCflags({
        "clang", // DO NOT REMOVE, first flag is ignored
        "-O2",
        "-D__KERNEL__",
        "-fno-color-diagnostics",
        "-fno-unwind-tables",
        "-fno-asynchronous-unwind-tables",
        "-fno-stack-protector",
        "-x", "c"
    }),
    theTriple("bpf")
{
    if (!ClangCompiler::llvmInitialized) {
        LLVMInitializeBPFTarget();
        LLVMInitializeBPFTargetMC();
        LLVMInitializeBPFTargetInfo();
        LLVMInitializeBPFAsmPrinter();
        LLVMInitializeBPFAsmParser();

        ClangCompiler::llvmInitialized = true;
    }

    theDriver = std::make_unique<clang::driver::Driver>(
        name, getArch(), *diagnosticsEngine,
        llvm::vfs::getRealFileSystem()
    );

    std::string arch = "bpf";
    std::string Error;

    theTarget = llvm::TargetRegistry::lookupTarget(theTriple.getTriple(), Error);
    if (!theTarget) {
        throw Error;
    }

    llvm::TargetOptions targetOptions;
    auto RM = llvm::Optional<llvm::Reloc::Model>();
    targetMachine = std::unique_ptr<llvm::TargetMachine>(theTarget->createTargetMachine(
        theTriple.getTriple(), "generic", "", targetOptions, RM, llvm::None, llvm::CodeGenOpt::Aggressive));

    if (!targetMachine) {
        throw std::string("Could not allocate target machine");
    }
}

std::unique_ptr<clang::CompilerInvocation> ClangCompiler::buildCompilation(
    const char *inputFile,
    const char *outputFile,
    const std::vector<const char*> &extraCflags,
    bool verbose)
{
    std::vector<const char*> cflags;
    for (auto it = defaultCflags.begin(); it != defaultCflags.end(); it++)
        cflags.push_back(*it);
    for (auto it = extraCflags.begin(); it != extraCflags.end(); it++)
        cflags.push_back(*it);

    if (verbose) {
        cflags.push_back("-v");
    }

    cflags.push_back("-c");
    cflags.push_back(inputFile);

    if (outputFile) {
        cflags.push_back("-o");
        cflags.push_back(outputFile);
    }

    // Build
    std::unique_ptr<clang::driver::Compilation> compilation(theDriver->BuildCompilation(cflags));

    // expect exactly 1 job, otherwise error
    const clang::driver::JobList &jobs = compilation->getJobs();
    if (jobs.size() != 1 || !clang::isa<clang::driver::Command>(*jobs.begin())) {
        clang::SmallString<256> msg;
        llvm::raw_svector_ostream os(msg);
        jobs.Print(os, "; ", true);
        diagnosticsEngine->Report(clang::diag::err_fe_expected_compiler_job) << os.str();
        return nullptr;
    }

    const clang::driver::Command &cmd = clang::cast<clang::driver::Command>(*jobs.begin());
    if (llvm::StringRef(cmd.getCreator().getName()) != "clang") {
        diagnosticsEngine->Report(clang::diag::err_fe_expected_clang_command);
        return nullptr;
    }

    if (compilation->containsError()) {
        return nullptr;
    }

    if (verbose) {
        llvm::errs() << "clang invocation:\n";
        jobs.Print(llvm::errs(), "\n", true);
        llvm::errs() << "\n";
    }

    std::unique_ptr<clang::CompilerInvocation> invocation(new clang::CompilerInvocation);
    const llvm::opt::ArgStringList &ccargs = cmd.getArguments();

#if LLVM_MAJOR_VERSION >= 10
    clang::CompilerInvocation::CreateFromArgs(*invocation, ccargs, *diagnosticsEngine);
#else
    clang::CompilerInvocation::CreateFromArgs(
        *invocation, const_cast<const char **>(ccargs.data()),
        const_cast<const char **>(ccargs.data()) + ccargs.size(), *diagnosticsEngine);
#endif

    return invocation;
}

std::unique_ptr<llvm::Module> ClangCompiler::compileToBytecode(
    const char *inputFile,
    const char *outputFile,
    const std::vector<const char*> &cflags,
    bool verbose)
{
    auto invocation = buildCompilation(inputFile, outputFile, cflags, verbose);
    if (!invocation) {
        return nullptr;
    }


    if (outputFile) {
        invocation->getFrontendOpts().OutputFile = std::string(llvm::StringRef(outputFile));
    }

    invocation->getFrontendOpts().ProgramAction = clang::frontend::EmitLLVM;

    clang::CompilerInstance compiler;
    compiler.setInvocation(std::move(invocation));

    compiler.createDiagnostics();
    if (!compiler.hasDiagnostics()) {
        return nullptr;
    }

    std::unique_ptr<clang::CodeGenAction> emitLLVMAction(new clang::EmitLLVMAction(llvmContext.get()));
    if (!compiler.ExecuteAction(*emitLLVMAction)) {
        return nullptr;
    }

    return emitLLVMAction->takeModule();
}

llvm::StringRef ClangCompiler::getDataLayout()
{
#if LLVM_MAJOR_VERSION >= 11
#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
    return "e-m:e-p:64:64-i64:64-i128:128-n32:64-S128";
#else
    return "E-m:e-p:64:64-i64:64-i128:128-n32:64-S128";
#endif
#else
#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
    return "e-m:e-p:64:64-i64:64-n32:64-S128";
#else
    return "E-m:e-p:64:64-i64:64-n32:64-S128";
#endif
#endif
}

llvm::StringRef ClangCompiler::getArch() {
    Architecture arch = Architecture::PPCLE;

    const char *archenv = getenv("ARCH");
    if (archenv == NULL) {
#if defined(__powerpc64__)
#if defined(_CALL_ELF) && _CALL_ELF == 2
        arch = Architecture::PPCLE;
#else
        arch = Architecture::PPC;
    #endif
#elif defined(__s390x__)
        arch = Architecture::S390X;
#elif defined(__aarch64__)
        arch = Architecture::ARM64;
#else
        arch = Architecture::X86;
#endif
    } else if (!strcmp(archenv, "powerpc")) {
#if defined(_CALL_ELF) && _CALL_ELF == 2
        arch = Architecture::PPCLE;
#else
        arch = Architecture::PPC;
#endif
    } else if (!strcmp(archenv, "s390x")) {
        arch = Architecture::S390X;
    } else if (!strcmp(archenv, "arm64")) {
        arch = Architecture::ARM64;
    } else {
        arch = Architecture::X86;
    }

    switch(arch) {
    case Architecture::PPCLE:
        return "powerpc64le-unknown-linux-gnu";
    case Architecture::PPC:
        return "powerpc64-unknown-linux-gnu";
    case Architecture::S390X:
        return "s390x-ibm-linux-gnu";
    case Architecture::ARM64:
        return "aarch64-unknown-linux-gnu";
    default:
        return "x86_64-unknown-linux-gnu";
    }
}

int ClangCompiler::bytecodeToObjectFile(llvm::Module *module, const char *outputFile)
{
    module->setDataLayout(getDataLayout());
    module->setTargetTriple(theTriple.getTriple());

    std::error_code EC;
    llvm::raw_fd_ostream dest(outputFile, EC, llvm::sys::fs::OF_None);

    if (EC) {
        llvm::errs() << "Could not open file: " << EC.message();
        return -1;
    }

    llvm::legacy::PassManager pass;
    if (targetMachine->addPassesToEmitFile(pass, dest, nullptr, llvm::CGFT_ObjectFile)) {
        llvm::errs() << "TargetMachine can't emit a file of this type";
        return -1;
    }

    pass.run(*module);
    dest.flush();

    return 0;
}

const std::string& ClangCompiler::getErrors() {
    return errString;
}

ClangCompiler::~ClangCompiler() {
}
