// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

#include "rtloader.h"
#include "memory.h"

#ifndef _WIN32
// clang-format off
// handler stuff
#include <execinfo.h>
#include <csignal>
#include <cstring>
#include <sys/types.h>
#include <unistd.h>

// logging to cerr
#include <iostream>
#include <sstream>
#include <errno.h>
// clang-format on

core_trigger_t core_dump = NULL;

static inline void core(int sig)
{
    signal(sig, SIG_DFL);
    kill(getpid(), sig);
}

//! signalHandler
/*!
  \brief Crash handler for UNIX OSes
  \param sig Integer representing the signal number that triggered the crash.
  \param Unused siginfo_t parameter.
  \param Unused void * pointer parameter.

  This crash handler intercepts crashes triggered in C-land, printing the stacktrace
  at the time of the crash to stderr - logging cannot be assumed to be working at this
  poinrt and hence the use of stderr. If the core dump has been enabled, we will also
  dump a core - of course the correct ulimits need to be set for the dump to be created.
  The idea of handling the crashes here is to allow us to collect the stacktrace, with
  all its C-context, before it unwinds as would be the case if we allowed the go runtime
  to handle it.
*/
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

        _free(symbols);
    }

    // dump core if so configured
    __sync_synchronize();
    if (core_dump) {
        core_dump(sig);
    } else {
        kill(getpid(), SIGABRT);
    }
}

bool RtLoader::handleCrashes(const bool coredump) const
{
    // register signal handlers
    struct sigaction sa;
    sa.sa_flags = SA_SIGINFO;
    sa.sa_sigaction = signalHandler;

    // on segfault - what else?
    int err = sigaction(SIGSEGV, &sa, NULL);

    if (coredump && err == 0) {
        __sync_synchronize();
        core_dump = core;
    }
    if (err) {
        std::stringstream ss;
        ss << "unable to set crash handler: " << strerror(errno);

        setError(ss.str());
    }

    return bool(err == 0);
}

#endif

void RtLoader::setError(const std::string &msg) const
{
    _errorFlag = true;
    _error = msg;
}

void RtLoader::setError(const char *msg) const
{
    _errorFlag = true;
    _error = msg;
}

const char *RtLoader::getError() const
{
    if (!_errorFlag) {
        // error was already fetched, cleanup
        _error = "";
    } else {
        _errorFlag = false;
    }

    return _error.c_str();
}

bool RtLoader::hasError() const
{
    return _errorFlag;
}

void RtLoader::clearError()
{
    _errorFlag = false;
    _error = "";
}

void RtLoader::free(void *ptr)
{
    if (ptr != NULL) {
        _free(ptr);
    }
}
