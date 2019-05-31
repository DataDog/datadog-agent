// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

#include <six.h>

#ifndef _WIN32
// clang-format off
// handler stuff
#include <execinfo.h>
#include <csignal>
#include <sys/types.h>
#include <unistd.h>

// logging to cerr
#include <iostream>
// clang-format on

core_trigger_t core_dump = NULL;

static inline void core(int sig)
{
    signal(sig, SIG_DFL);
    kill(getpid(), sig);
}

#    define STACKTRACE_SIZE 500
void signalHandler(int sig, siginfo_t *, void *)
{
    void *buffer[STACKTRACE_SIZE];
    char **symbols;

    size_t nptrs = backtrace(buffer, STACKTRACE_SIZE);
    std::cerr << "HANDLER CAUGHT signal Error: signal " << sig << std::endl;
    symbols = backtrace_symbols(buffer, nptrs);
    if (symbols == NULL) {
        std::cerr << "Error getting backtrace symbols" << std::endl;
    } else {
        std::cerr << "C-LAND STACKTRACE: " << std::endl;
        for (int i = 0; i < nptrs; i++) {
            std::cerr << symbols[i] << std::endl;
        }

        free(symbols);
    }

    // dump core if so configured
    __sync_synchronize();
    if (core_dump) {
        core_dump(sig);
    } else {
        kill(getpid(), SIGABRT);
    }
}

void Six::handleCrashes(const bool coredump) const
{
    // register signal handlers
    struct sigaction sa;
    sa.sa_flags = SA_SIGINFO;
    sa.sa_sigaction = signalHandler;

    // on segfault - what else?
    sigaction(SIGSEGV, &sa, NULL);

    if (coredump) {
        __sync_synchronize();
        core_dump = core;
    }
}

#endif

void Six::setError(const std::string &msg) const
{
    _errorFlag = true;
    _error = msg;
}

void Six::setError(const char *msg) const
{
    _errorFlag = true;
    _error = msg;
}

const char *Six::getError() const
{
    if (!_errorFlag) {
        // error was already fetched, cleanup
        _error = "";
    } else {
        _errorFlag = false;
    }

    return _error.c_str();
}

bool Six::hasError() const
{
    return _errorFlag;
}

void Six::clearError()
{
    _errorFlag = false;
    _error = "";
}

void Six::free(void *ptr)
{
    if (ptr != NULL) {
        ::free(ptr);
    }
}
