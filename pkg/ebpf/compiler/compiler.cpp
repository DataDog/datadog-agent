#include "compiler.h"
#include "files.h"

#include <iostream>
#include <mutex>
#include <llvm/IR/LegacyPassManager.h>
#include <llvm/Support/TargetSelect.h>
#include <clang/CodeGen/CodeGenAction.h>
#include <clang/Driver/Job.h>
#include <clang/Driver/Tool.h>
#include <clang/Frontend/CompilerInstance.h>
#include <clang/Lex/PreprocessorOptions.h>

std::once_flag ClangCompiler::llvmInitialized;
std::map<std::string, std::unique_ptr<llvm::MemoryBuffer>> ClangCompiler::remapped_files;
const std::string ClangCompiler::main_path = "/virtual/main.c";

enum Architecture { PPC, PPCLE, S390X, ARM64, X86 };

ClangCompiler::ClangCompiler(const char *name) :
    llvmContext(std::make_unique<llvm::LLVMContext>()),
    diagOpts(new clang::DiagnosticOptions()),
    errStream(errString),
    textDiagnosticPrinter(std::make_unique<clang::TextDiagnosticPrinter>(errStream, diagOpts.get())),
    diagnosticsEngine(new clang::DiagnosticsEngine(diagID, diagOpts, textDiagnosticPrinter.get(), false)),
    defaultCflags({
        "clang", // DO NOT REMOVE, first flag is ignored
        "-emit-llvm",
        "-O2",
        "-D__KERNEL__",
        "-fno-color-diagnostics",
        "-fno-unwind-tables",
        "-fno-asynchronous-unwind-tables",
        "-fno-stack-protector",
        "-nostdinc",
        "-isystem/virtual/lib/clang/include",
        "-x", "c"
    }),
    theTriple("bpf")
{
    std::call_once(llvmInitialized, []{
        LLVMInitializeBPFTarget();
        LLVMInitializeBPFTargetMC();
        LLVMInitializeBPFTargetInfo();
        LLVMInitializeBPFAsmPrinter();
        LLVMInitializeBPFAsmParser();

        for (auto f : MappedFiles::files) {
            remapped_files[f.first] = llvm::MemoryBuffer::getMemBuffer(f.second);
        }
    });

    theDriver = std::make_unique<clang::driver::Driver>(
        name, getArch(), *diagnosticsEngine,
        llvm::vfs::getRealFileSystem()
    );

    std::string arch = "bpf";
    std::string Error;

    theTarget = llvm::TargetRegistry::lookupTarget(theTriple.getTriple(), Error);
    if (!theTarget) {
        errString = "could not lookup target";
        return;
    }

    llvm::TargetOptions targetOptions;
    auto RM = llvm::Optional<llvm::Reloc::Model>();
    targetMachine = std::unique_ptr<llvm::TargetMachine>(theTarget->createTargetMachine(
        theTriple.getTriple(), "generic", "", targetOptions, RM, llvm::None, llvm::CodeGenOpt::Aggressive));

    if (!targetMachine) {
        errString = "could not allocate target machine";
        return;
    }

    diagnosticsEngine->setSuppressSystemWarnings(true);
}

std::unique_ptr<clang::CompilerInvocation> ClangCompiler::buildCompilation(
    const char *inputFile,
    const char *outputFile,
    const std::vector<const char*> &extraCflags,
    bool verbose,
    bool inMemory)
{
    auto cflags = defaultCflags;
    for (auto it = extraCflags.begin(); it != extraCflags.end(); it++) {
        cflags.push_back(*it);
    }

    if (verbose) {
        cflags.push_back("-v");
    }

    cflags.push_back("-x");
    cflags.push_back("c");
    cflags.push_back("-c");
    cflags.push_back(inputFile);

    if (outputFile) {
        cflags.push_back("-o");
        cflags.push_back(outputFile);
    }

    // Build
    if (inMemory) {
        theDriver->setCheckInputsExist(false);
    }
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
        errStream << "clang invocation:\n";
        jobs.Print(errStream, "\n", true);
        errStream << "\n";
    }

    auto invocation = std::make_unique<clang::CompilerInvocation>();
    const llvm::opt::ArgStringList &ccargs = cmd.getArguments();

    clang::CompilerInvocation::CreateFromArgs(*invocation, ccargs, *diagnosticsEngine);
    return invocation;
}

std::unique_ptr<llvm::Module> ClangCompiler::compileToBytecode(
    const char *input,
    const char *outputFile,
    const std::vector<const char*> &cflags,
    bool verbose,
    bool inMemory)
{
    std::unique_ptr<llvm::MemoryBuffer> main_buf;
    const char *inputFile;

    if (inMemory) {
        inputFile = main_path.c_str();
    } else {
        inputFile = input;
    }

    auto invocation = buildCompilation(inputFile, outputFile, cflags, verbose, inMemory);
    if (!invocation) {
        return nullptr;
    }

    invocation->getPreprocessorOpts().RetainRemappedFileBuffers = true;
    if (inMemory) {
        main_buf = llvm::MemoryBuffer::getMemBuffer(input);
        invocation->getPreprocessorOpts().addRemappedFile(main_path, &*main_buf);
    }
    for (const auto &f : remapped_files) {
        invocation->getPreprocessorOpts().addRemappedFile(f.first, &*f.second);
    }

    if (outputFile) {
        invocation->getFrontendOpts().OutputFile = std::string(llvm::StringRef(outputFile));
    }

    invocation->getFrontendOpts().ProgramAction = clang::frontend::EmitLLVM;
    invocation->getFrontendOpts().DisableFree = false;
    invocation->getCodeGenOpts().DisableFree = false;

    clang::CompilerInstance compiler;
    compiler.setInvocation(std::move(invocation));
    compiler.setDiagnostics(diagnosticsEngine.get());

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
#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
    return "e-m:e-p:64:64-i64:64-i128:128-n32:64-S128";
#else
    return "E-m:e-p:64:64-i64:64-i128:128-n32:64-S128";
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

int ClangCompiler::bytecodeToObjectFile(llvm::Module &module, const char *outputFile)
{
    if (!outputFile) {
        diagnosticsEngine->Report(clang::diag::err_cannot_open_file) << "Invalid output file";
        return -1;
    }

    module.setDataLayout(getDataLayout());
    module.setTargetTriple(theTriple.getTriple());

    std::error_code EC;
    llvm::raw_fd_ostream dest(outputFile, EC, llvm::sys::fs::OF_None);

    if (EC) {
        diagnosticsEngine->Report(clang::diag::err_cannot_open_file) << EC.message();
        return -1;
    }

    llvm::legacy::PassManager pass;
    if (targetMachine->addPassesToEmitFile(pass, dest, nullptr, llvm::CGFT_ObjectFile)) {
        diagnosticsEngine->Report(clang::diag::err_fe_unable_to_create_target) << "TargetMachine can't emit a file of this type";
        return -1;
    }

    pass.run(module);
    dest.flush();

    return 0;
}

const std::string& ClangCompiler::getErrors() const
{
    return errString;
}

ClangCompiler::~ClangCompiler()
{
}
