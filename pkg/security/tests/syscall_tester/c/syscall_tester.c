#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/ptrace.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <string.h>
#include <stdint.h>
#include <sys/ioctl.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/fsuid.h>
#include <sys/mman.h>
#include <fcntl.h>
#include <pthread.h>
#include <signal.h>
#include <errno.h>
#include <arpa/inet.h>
#include <linux/un.h>
#include <err.h>
#include <limits.h>
#include <sys/time.h>
#include <sys/resource.h>

#define RPC_CMD 0xdeadc001
#define REGISTER_SPAN_TLS_OP 6

#ifndef SYS_gettid
#error "SYS_gettid unavailable on this system"
#endif

pid_t gettid(void) {
    pid_t tid = syscall(SYS_gettid);
    return tid;
}

struct span_tls_t {
    uint64_t format;
    uint64_t max_threads;
    void *base;
};

struct thread_opts {
    struct span_tls_t *tls;
    char **argv;
};

void *register_tls() {
    uint64_t max_threads = 100;
    uint64_t len = max_threads * (sizeof(uint64_t) + sizeof(__int128));

    void *base = (void *)malloc(len);
    if (base == NULL)
        return NULL;
    bzero(base, len);

    struct span_tls_t *tls = (struct span_tls_t *)malloc(sizeof(struct span_tls_t));
    if (tls == NULL)
        return NULL;
    tls->max_threads = max_threads;
    tls->base = base;
    tls->format = 0; // format is not needed

    uint8_t request[257];
    bzero(request, sizeof(request));

    request[0] = REGISTER_SPAN_TLS_OP;
    memcpy(&request[1], tls, sizeof(struct span_tls_t));
    ioctl(0, RPC_CMD, &request);

    return tls;
}

void register_span(struct span_tls_t *tls, __int128 trace_id, unsigned long span_id) {
    int offset = (gettid() % tls->max_threads) * 24; // sizeof uint64 + sizeof int128

    *(uint64_t*)(tls->base + offset) = span_id;
    *(__int128*)(tls->base + offset + 8) = trace_id;
}

__int128 atouint128(char *s) {
    if (s == NULL)
        return (0);

    __int128_t val = 0;
    for (; *s != 0 && *s >= '0' && *s <= '9'; s++) {
        val = (10 * val) + (*s - '0');
    }
    return val;
}

static void *thread_span_exec(void *data) {
    struct thread_opts *opts = (struct thread_opts *)data;

    __int128_t trace_id = atouint128(opts->argv[1]);
    unsigned span_id = atoi(opts->argv[2]);

    register_span(opts->tls, trace_id, span_id);

    execv(opts->argv[3], opts->argv + 3);
    return NULL;
}

int span_exec(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Please pass a span Id and a trace Id to exec_span and a command\n");
        return EXIT_FAILURE;
    }

    struct span_tls_t *tls = register_tls();
    if (!tls) {
        fprintf(stderr, "Failed to register TLS\n");
        return EXIT_FAILURE;
    }

    struct thread_opts opts = {
        .argv = argv,
        .tls = tls,
    };

    pthread_t thread;
    if (pthread_create(&thread, NULL, thread_span_exec, &opts) < 0) {
        return EXIT_FAILURE;
    }
    pthread_join(thread, NULL);

    return EXIT_SUCCESS;
}

static void *thread_open(void *data) {
    struct thread_opts *opts = (struct thread_opts *)data;

    __int128_t trace_id = atouint128(opts->argv[1]);
    unsigned span_id = atoi(opts->argv[2]);

    register_span(opts->tls, trace_id, span_id);

    int fd = open(opts->argv[3], O_CREAT);
    if (fd < 0) {
        fprintf(stderr, "Unable to create file `%s`\n", opts->argv[3]);
        return NULL;
    }
    close(fd);
    unlink(opts->argv[3]);

    return NULL;
}

int span_open(int argc, char **argv) {
    if (argc < 4) {
        fprintf(stderr, "Please pass a span Id, a trace Id and a file path to span-open\n");
        return EXIT_FAILURE;
    }

    struct span_tls_t *tls = register_tls();
    if (!tls) {
        fprintf(stderr, "Failed to register TLS\n");
        return EXIT_FAILURE;
    }

    struct thread_opts opts = {
        .argv = argv,
        .tls = tls,
    };

    pthread_t thread;
    if (pthread_create(&thread, NULL, thread_open, &opts) < 0) {
        return EXIT_FAILURE;
    }
    pthread_join(thread, NULL);

    return EXIT_SUCCESS;
}

int ptrace_traceme() {
    int child = fork();
    if (child == 0) {
        ptrace(PTRACE_TRACEME, 0, NULL, NULL);
        raise(SIGSTOP);
    } else {
        wait(NULL);
        ptrace(PTRACE_CONT, child, 42, NULL);
    }
    return EXIT_SUCCESS;
}

int ptrace_attach() {
    int child = fork();
    if (child == 0) {
        sleep(3);
    } else {
        ptrace(PTRACE_ATTACH, child, 0, NULL);
        wait(NULL);
        sleep(3); // sleep here to let the agent resolve the pid namespace on procfs
    }
    return EXIT_SUCCESS;
}

int setrlimit_nofile() {
    struct rlimit rlim;
    rlim.rlim_cur = 1024;  // soft limit
    rlim.rlim_max = 2048;  // hard limit
    
    if (setrlimit(RLIMIT_NOFILE, &rlim) < 0) {
        perror("setrlimit RLIMIT_NOFILE");
        return EXIT_FAILURE;
    }
    return EXIT_SUCCESS;
}

int setrlimit_nproc() {
    struct rlimit rlim;
    rlim.rlim_cur = 512;   // soft limit
    rlim.rlim_max = 1024;  // hard limit
    
    if (setrlimit(RLIMIT_NPROC, &rlim) < 0) {
        perror("setrlimit RLIMIT_NPROC");
        return EXIT_FAILURE;
    }
    return EXIT_SUCCESS;
}

int prlimit64_stack(void) {
    struct rlimit64 rlim;
    rlim.rlim_cur = 1024;   
    rlim.rlim_max = 2048;

    pid_t dummy_pid = fork();
    if (dummy_pid < 0) {
        perror("fork");
        return EXIT_FAILURE;
    }

    if (dummy_pid == 0) {
        sleep(30);
        return EXIT_SUCCESS;
    }

    if (prlimit64(dummy_pid, RLIMIT_STACK, &rlim, NULL) < 0) {
        perror("prlimit64 RLIMIT_STACK");
        kill(dummy_pid, SIGTERM);
        waitpid(dummy_pid, NULL, 0);
        return EXIT_FAILURE;
    }

    kill(dummy_pid, SIGTERM);
    waitpid(dummy_pid, NULL, 0);
    return EXIT_SUCCESS;
}

int setrlimit_core() {
    struct rlimit rlim;
    rlim.rlim_cur = 0;      // no core dumps
    rlim.rlim_max = 0;      // no core dumps
    
    if (setrlimit(RLIMIT_CORE, &rlim) < 0) {
        perror("setrlimit RLIMIT_CORE");
        return EXIT_FAILURE;
    }    
    return EXIT_SUCCESS;
}

int test_signal_sigusr(int child, int sig) {
    int do_fork = child == 0;
    if (do_fork) {
        child = fork();
        if (child == 0) {
            sleep(5);
            return EXIT_SUCCESS;
        }
    }

    int ret = kill(child, sig);
    if (ret < 0) {
        return ret;
    }

    if (do_fork)
        wait(NULL);

    return ret;
}

int test_signal_eperm(void) {
    int ppid = getpid();
    int child = fork();
    if (child == 0) {
        /* switch to user daemon */
        if (setuid(1)) {
            fprintf(stderr, "Failed to setuid 1 (%s)\n", strerror(errno));
            return EXIT_FAILURE;
        }
        kill(ppid, SIGKILL);
        sleep(1);
    } else {
        wait(NULL);
    }
    return EXIT_SUCCESS;
}

int test_signal(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "%s: Please pass a test case in: sigusr, eperm, and an optional pid.\n", __FUNCTION__);
        return EXIT_FAILURE;
    }

    int pid = 0;
    if (argc > 2) {
        pid = atoi(argv[2]);
        if (pid < 1) {
            fprintf(stderr, "invalid pid: %s\n", argv[2]);
            return EXIT_FAILURE;
        }
    }

    if (!strcmp(argv[1], "sigusr1"))
        return test_signal_sigusr(pid, SIGUSR1);
    else if (!strcmp(argv[1], "sigusr2"))
        return test_signal_sigusr(pid, SIGUSR2);
    else if (!strcmp(argv[1], "eperm"))
        return test_signal_eperm();
    fprintf(stderr, "%s: Unknown argument: %s.\n", __FUNCTION__, argv[1]);
    return EXIT_FAILURE;
}

int test_splice() {
    const int fd = open("/tmp/splice_test", O_RDONLY | O_CREAT, 0700);
    if (fd < 0) {
        fprintf(stderr, "open failed");
        return EXIT_FAILURE;
    }

    int p[2];
    if (pipe(p)) {
        fprintf(stderr, "pipe failed");
        return EXIT_FAILURE;
    }

    loff_t offset = 1;
    splice(fd, 0, p[1], NULL, 1, 0);
    close(fd);
    sleep(5);
    remove("/tmp/splice_test");

    return EXIT_SUCCESS;
}

int test_setregid(int argc, char **argv) {
    if (setregid(1, 1) != 0) {
        fprintf(stderr, "setregid failed");
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int test_setreuid(int argc, char **argv) {
    if (setreuid(1, 1) != 0) {
        fprintf(stderr, "setreuid failed");
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int test_mkdirat(int argc, char **argv) {
    if (argc != 2) {
        fprintf(stderr, "%s: Please pass a path to mkdirat.\n", __FUNCTION__);
        return EXIT_FAILURE;
    }

    return mkdirat(0, argv[1], 0777);
}

int test_mkdirat_error(int argc, char **argv) {
    int ret = test_setregid(argc, argv);
    if (ret)
        return ret;

    ret = test_setreuid(argc, argv);
    if (ret)
        return ret;

    if ((ret = test_mkdirat(argc, argv)) == 0) {
        fprintf(stderr, "mkdirat succeeded even though we expected it to fail");
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int test_process_set(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "%s: Please pass a syscall name, real and effective id.\n", __FUNCTION__);
        return EXIT_FAILURE;
    }

    int real_id = atoi(argv[2]);
    int effective_id = atoi(argv[3]);

    char *subcmd = argv[1];

    int res;
    if (strcmp(subcmd, "setuid") == 0) {
        res = setuid(real_id);
    } else if (strcmp(subcmd, "setreuid") == 0) {
        res = setreuid(real_id, effective_id);
    } else if (strcmp(subcmd, "setresuid") == 0) {
        res = setresuid(real_id, effective_id, 0);
    } else if (strcmp(subcmd, "setfsuid") == 0) {
        res = setfsuid(real_id);
    } else if (strcmp(subcmd, "setgid") == 0) {
        res = setgid(real_id);
    } else if (strcmp(subcmd, "setregid") == 0) {
        res = setregid(real_id, effective_id);
    } else if (strcmp(subcmd, "setresgid") == 0) {
        res = setresgid(real_id, effective_id, 0);
    } else if (strcmp(subcmd, "setfsgid") == 0) {
        res = setfsgid(real_id);
    } else {
        fprintf(stderr, "Unknown subcommand `%s`\n", subcmd);
        return EXIT_FAILURE;
    }

    if (res != 0) {
        fprintf(stderr, "%s failed", subcmd);
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int self_exec(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Please pass a command name\n");
        return EXIT_FAILURE;
    }

    execv("/proc/self/exe", argv + 1);

    return EXIT_SUCCESS;
}

void* connect_thread_ipv4(void *arg) {
    int s = socket(PF_INET, SOCK_STREAM, IPPROTO_TCP);
    connect(s, (struct sockaddr*)arg, sizeof(struct sockaddr));
    return NULL;
}

int test_accept_af_inet(int argc, char** argv) {
    pthread_t thread;

    if (argc != 5) {
        fprintf(stderr, "%s: please specify a valid command:\n", __FUNCTION__);
        fprintf(stderr, "Arg1: IP address where the socket should bind to\n");
        fprintf(stderr, "Arg2: IP address where the socket should connect to\n");
        fprintf(stderr, "Arg3: Port to bind\n");
        fprintf(stderr, "Arg4: Pass sockaddr_in <true/false>\n");
        return EXIT_FAILURE;
    }

    const char* bind_to = argv[1];
    const char* connect_to = argv[2];
    int port = atoi(argv[3]);

    struct sockaddr_in *sockAddrPtr = NULL;
    struct sockaddr_in sockAddr;
    memset(&sockAddr, 0, sizeof(struct sockaddr_in));

    socklen_t sockLen = sizeof(struct sockaddr_in);

    if (strcmp(argv[4], "true") == 0) {
        sockAddrPtr = &sockAddr;
    }

    int s;
    s = socket(PF_INET, SOCK_STREAM, IPPROTO_TCP);

    if (s < 0) {
        perror("socket");
        return EXIT_FAILURE;
    }

    int ip32 = 0;

    struct sockaddr_in bindAddr;
    memset(&bindAddr, 0, sizeof(struct sockaddr_in));
    bindAddr.sin_family = AF_INET;
    if (inet_pton(AF_INET, bind_to, &ip32) != 1) {
        perror("inet_pton bind_to");
        return EXIT_FAILURE;
    }

    bindAddr.sin_addr.s_addr = htonl(ip32);
    bindAddr.sin_port = htons(port);

    struct sockaddr_in connectAddr;
    memset(&connectAddr, 0, sizeof(struct sockaddr_in));
    connectAddr.sin_family = AF_INET;
    if (inet_pton(AF_INET, connect_to, &ip32) != 1) {
        perror("inet_pton connect_to");
        return EXIT_FAILURE;
    }

    connectAddr.sin_addr.s_addr = ip32;
    connectAddr.sin_port = htons(port);

    if (bind(s, (struct sockaddr*)&bindAddr, sizeof(struct sockaddr)) < 0) {
        close(s);
        perror("Failed to bind");
        return EXIT_FAILURE;
    }

    if (listen(s, 10) < 0) {
        close(s);
        perror("Failed to listen");
        return EXIT_FAILURE;
    }

    pthread_create(&thread, NULL, connect_thread_ipv4, (void*)&connectAddr);

    if (accept(s, (struct sockaddr*)sockAddrPtr, &sockLen) < 0) {
        perror("Failed to accept");
    }

    close(s);
    pthread_join(thread, NULL);
    return EXIT_SUCCESS;
}

void* connect_thread_ipv6(void *arg) {
    int s = socket(PF_INET6, SOCK_STREAM, IPPROTO_TCP);
    connect(s, (struct sockaddr_in6*)arg, sizeof(struct sockaddr_in6));

    return NULL;
}

int test_accept_af_inet6(int argc, char** argv) {
    pthread_t thread;

    if (argc != 5) {
        fprintf(stderr, "%s: please specify a valid command:\n", __FUNCTION__);
        fprintf(stderr, "Arg1: IP address where the socket should bind to\n");
        fprintf(stderr, "Arg2: IP address where the socket should connect to\n");
        fprintf(stderr, "Arg3: Port to bind\n");
        fprintf(stderr, "Arg4: Pass sockaddr_in <true/false>\n");
        return EXIT_FAILURE;
    }

    const char* bind_to = argv[1];
    const char* connect_to = argv[2];
    int port = atoi(argv[3]);

    struct sockaddr_in6 *sockAddrPtr = NULL;
    struct sockaddr_in6 sockAddr;
    memset(&sockAddr, 0, sizeof(struct sockaddr_in6));

    socklen_t sockLen = sizeof(struct sockaddr_in6);

    if (strcmp(argv[4], "true") == 0) {
        sockAddrPtr = &sockAddr;
    }

    int s;
    s = socket(PF_INET6, SOCK_STREAM, IPPROTO_TCP);

    if (s < 0) {
        perror("socket");
        return EXIT_FAILURE;
    }

    struct in6_addr ip6;

    struct sockaddr_in6 bindAddr;
    memset(&bindAddr, 0, sizeof(struct sockaddr_in6));
    bindAddr.sin6_family = AF_INET6;
    if (inet_pton(AF_INET6, bind_to, &ip6) != 1) {
        perror("inet_pton bind_to");
        return EXIT_FAILURE;
    }
    bindAddr.sin6_addr = ip6;
    bindAddr.sin6_port = htons(port);

    struct sockaddr_in6 connectAddr;
    memset(&connectAddr, 0, sizeof(struct sockaddr_in6));
    connectAddr.sin6_family = AF_INET6;
    if (inet_pton(AF_INET6, connect_to, &ip6) != 1) {
        perror("inet_pton connect_to");
        return EXIT_FAILURE;
    }
    connectAddr.sin6_addr = ip6;
    connectAddr.sin6_port = htons(port);

    if (bind(s, &bindAddr, sizeof(struct sockaddr_in6)) < 0) {
        close(s);
        perror("Failed to bind");
        return EXIT_FAILURE;
    }

    if (listen(s, 10) < 0) {
        close(s);
        perror("Failed to listen");
        return EXIT_FAILURE;
    }

    pthread_create(&thread, NULL, connect_thread_ipv6, (void*)&connectAddr);

    if (accept(s, (struct sockaddr*)sockAddrPtr, &sockLen) < 0) {
        perror("Failed to accept");
    }

    pthread_join(thread, NULL);
    close (s);
    return EXIT_SUCCESS;
}

int test_accept(int argc, char** argv) {
    if (argc <= 2) {
        fprintf(stderr, "Please specify an addr_type\n");
        return EXIT_FAILURE;
    }

    if(strcmp(argv[1],"AF_INET") == 0) {
        return test_accept_af_inet(argc - 1, argv + 1);
    } else if(strcmp(argv[1], "AF_INET6") == 0) {
        return test_accept_af_inet6(argc - 1, argv + 1);
    }

    return EXIT_FAILURE;
}

int test_bind_af_inet(int argc, char** argv) {

    if (argc != 3) {
        fprintf(stderr, "%s: please specify a valid command:\n", __FUNCTION__);
        fprintf(stderr, "Arg1: an option for the addr in the list: any, custom_ip\n");
        fprintf(stderr, "Arg2: an option for the protocol in the list: tcp, udp\n");
        return EXIT_FAILURE;
    }

    char* proto = argv[2];
    int s;
    if (!strcmp(proto, "udp"))
        s = socket(PF_INET, SOCK_DGRAM, IPPROTO_UDP);
    else
        s = socket(PF_INET, SOCK_STREAM, IPPROTO_TCP);
    if (s < 0) {
        perror("socket");
        return EXIT_FAILURE;
    }

    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;

    char* ip = argv[1];
    if (!strcmp(ip, "any")) {
        addr.sin_addr.s_addr = htonl(INADDR_ANY);
    } else if (!strcmp(ip, "custom_ip")) {
        int ip32 = 0;
        if (inet_pton(AF_INET, "127.0.0.1", &ip32) != 1) {
            perror("inet_pton");
            return EXIT_FAILURE;
        }
        addr.sin_addr.s_addr = htonl(ip32);
    } else {
        fprintf(stderr, "Please specify an option in the list: any, broadcast, custom_ip\n");
        return EXIT_FAILURE;
    }

    addr.sin_port = htons(4242);
    if (bind(s, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        perror("Failed to bind port");
        return EXIT_FAILURE;
    }

    close (s);
    return EXIT_SUCCESS;
}

int test_bind_af_inet6(int argc, char** argv) {
    int s = socket(AF_INET6, SOCK_STREAM, 0);
    if (s < 0) {
        perror("socket");
        return EXIT_FAILURE;
    }

    if (argc != 2) {
        fprintf(stderr, "Please specify an option in the list: any, custom_ip\n");
        return EXIT_FAILURE;
    }

    struct sockaddr_in6 addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin6_family = AF_INET6;

    char* ip = argv[1];
    if (!strcmp(ip, "any")) {
        inet_pton(AF_INET6, "::", &addr.sin6_addr);
    } else if (!strcmp(ip, "custom_ip")) {
        inet_pton(AF_INET6, "1234:5678:90ab:cdef:0000:0000:1a1a:1337", &addr.sin6_addr);
    } else {
        fprintf(stderr, "Please specify an option in the list: any, broadcast, custom_ip\n");
        return EXIT_FAILURE;
    }

    addr.sin6_port = htons(4242);
    if (bind(s, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        perror("Failed to bind port");
        return EXIT_FAILURE;
    }

    close(s);
    return EXIT_SUCCESS;
}

#define TEST_BIND_AF_UNIX_SERVER_PATH "/tmp/test_bind_af_unix"
int test_bind_af_unix(void) {
    int s = socket(AF_UNIX, SOCK_STREAM, 0);
    if (s < 0) {
        perror("socket");
        return EXIT_FAILURE;
    }

    unlink(TEST_BIND_AF_UNIX_SERVER_PATH);
    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, TEST_BIND_AF_UNIX_SERVER_PATH, strlen(TEST_BIND_AF_UNIX_SERVER_PATH));
    int ret = bind(s, (struct sockaddr*)&addr, sizeof(addr));
    if (ret)
        perror("bind");

    close(s);
    unlink(TEST_BIND_AF_UNIX_SERVER_PATH);
    return EXIT_SUCCESS;
}

int test_bind(int argc, char** argv) {
    if (argc <= 1) {
        fprintf(stderr, "Please specify an addr_type\n");
        return EXIT_FAILURE;
    }

    char* addr_family = argv[1];
    if (!strcmp(addr_family, "AF_INET")) {
        return test_bind_af_inet(argc - 1, argv + 1);
    } else if  (!strcmp(addr_family, "AF_INET6")) {
        return test_bind_af_inet6(argc - 1, argv + 1);
    } else if  (!strcmp(addr_family, "AF_UNIX")) {
        return test_bind_af_unix();
    }

    fprintf(stderr, "Specified %s addr_type is not a valid one, try: AF_INET, AF_INET6 or AF_UNIX\n", addr_family);
    return EXIT_FAILURE;
}

int test_connect_af_inet(int argc, char** argv) {

    if (argc != 3) {
        fprintf(stderr, "%s: please specify a valid command:\n", __FUNCTION__);
        fprintf(stderr, "Arg1: an option for the addr in the list: any, custom_ip\n");
        fprintf(stderr, "Arg2: an option for the protocol in the list: tcp, udp\n");
        return EXIT_FAILURE;
    }

    char* proto = argv[2];
    int s;

    if (!strcmp(proto, "udp"))
        s = socket(PF_INET, SOCK_DGRAM, IPPROTO_UDP);
    else
        s = socket(PF_INET, SOCK_STREAM, IPPROTO_TCP);

    if (s < 0) {
        perror("socket");
        return EXIT_FAILURE;
    }

    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;

    char* ip = argv[1];
    if (!strcmp(ip, "any")) {
        addr.sin_addr.s_addr = htonl(INADDR_ANY);
    } else if (!strcmp(ip, "custom_ip")) {
        int ip32 = 0;
        if (inet_pton(AF_INET, "127.0.0.1", &ip32) != 1) {
            perror("inet_pton");
            return EXIT_FAILURE;
        }
        addr.sin_addr.s_addr = htonl(ip32);
    } else {
        fprintf(stderr, "Please specify an option in the list: any, broadcast, custom_ip\n");
        return EXIT_FAILURE;
    }

    addr.sin_port = htons(4242);

    if (connect(s, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        close(s);
        perror("Failed to connect to port");
        return EXIT_FAILURE;
    }

    close (s);
    return EXIT_SUCCESS;
}

int test_connect_af_inet6(int argc, char** argv) {

    if (argc != 3) {
        fprintf(stderr, "%s: please specify a valid command:\n", __FUNCTION__);
        fprintf(stderr, "Arg1: an option for the addr in the list: any, custom_ip\n");
        fprintf(stderr, "Arg2: an option for the protocol in the list: tcp, udp\n");
        return EXIT_FAILURE;
    }

    char* proto = argv[2];
    int s;

    if (!strcmp(proto, "udp"))
        s = socket(AF_INET6, SOCK_DGRAM, IPPROTO_UDP);
    else
        s = socket(AF_INET6, SOCK_STREAM, IPPROTO_TCP);

    if (s < 0) {
        perror("socket");
        return EXIT_FAILURE;
    }

    struct sockaddr_in6 addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin6_family = AF_INET6;

    char* ip = argv[1];
    if (!strcmp(ip, "any")) {
        inet_pton(AF_INET6, "::", &addr.sin6_addr);
    } else if (!strcmp(ip, "custom_ip")) {
        inet_pton(AF_INET6, "1234:5678:90ab:cdef:0000:0000:1a1a:1337", &addr.sin6_addr);
    } else {
        fprintf(stderr, "Please specify an option in the list: any, broadcast, custom_ip\n");
        return EXIT_FAILURE;
    }

    addr.sin6_port = htons(4242);
    if (connect(s, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        close(s);
        perror("Failed to connect to port");
        return EXIT_FAILURE;
    }

    close(s);
    return EXIT_SUCCESS;
}

int test_connect_af_unix(int argc, char** argv) {
    if (argc != 3) {
        fprintf(stderr, "%s: please specify a valid command:\n", __FUNCTION__);
        fprintf(stderr, "Arg1: the path of the UNIX socket to connect to\n");
        fprintf(stderr, "Arg2: an option for the protocol in the list: tcp, udp\n");
        return EXIT_FAILURE;
    }

    char *proto = argv[2];
    int s;
    if (!strcmp(proto, "tcp")) {
        s = socket(AF_UNIX, SOCK_STREAM, 0);
    } else if (!strcmp(proto, "udp")) {
        s = socket(AF_UNIX, SOCK_DGRAM, 0);
    } else {
        fprintf(stderr, "Please specify an option in the list: tcp, udp\n");
        return EXIT_FAILURE;
    }

    if (s < 0) {
        perror("socket");
        return EXIT_FAILURE;
    }

    char *socket_path = argv[1];
    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;

    if (strlen(socket_path) >= sizeof(addr.sun_path)) {
        close(s);
        fprintf(stderr, "Path too long for AF_UNIX socket\n");
        return EXIT_FAILURE;
    }
    strncpy(addr.sun_path, socket_path, sizeof(addr.sun_path) - 1);

    if (connect(s, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        close(s);
        perror("Failed to connect to AF_UNIX socket");
        return EXIT_FAILURE;
    }

    close(s);
    return EXIT_SUCCESS;
}


int test_connect(int argc, char** argv) {
    if (argc <= 1) {
        fprintf(stderr, "Please specify an addr_type\n");
        return EXIT_FAILURE;
    }

    char* addr_family = argv[1];
    if (!strcmp(addr_family, "AF_INET")) {
        return test_connect_af_inet(argc - 1, argv + 1);
    } else if  (!strcmp(addr_family, "AF_INET6")) {
        return test_connect_af_inet6(argc - 1, argv + 1);
    } else if (!strcmp(addr_family, "AF_UNIX")) {
        return test_connect_af_unix(argc - 1, argv + 1);
    }
    fprintf(stderr, "Specified %s addr_type is not a valid one, try: AF_INET or AF_INET6 \n", addr_family);
    return EXIT_FAILURE;
}

int test_forkexec(int argc, char **argv) {
    if (argc == 2) {
        char *subcmd = argv[1];
        if (strcmp(subcmd, "exec") == 0) {
            int child = fork();
            if (child == 0) {
                char *const args[] = {"syscall_tester", "fork", "mmap", NULL};
                execv("/proc/self/exe", args);
            } else if (child > 0) {
                wait(NULL);
            }
            return EXIT_SUCCESS;
        } else if (strcmp(subcmd, "mmap") == 0) {
            open("/dev/null", O_RDONLY);
            return EXIT_SUCCESS;
        }
    } else if (argc == 1) {
        int child = fork();
        if (child == 0) {
            open("/dev/null", O_RDONLY);
            return EXIT_SUCCESS;
        } else if (child > 0) {
            wait(NULL);
        }
        return EXIT_SUCCESS;
    } else {
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int test_getchar(int argc, char **argv) {
    getchar();
    return EXIT_SUCCESS;
}

int test_open(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Please specify at least a file name \n");
        return EXIT_FAILURE;
    }

    for (int i = 1; i != argc; i++) {
        char *filename = argv[i];
        int fd = open(filename, O_RDONLY | O_CREAT, 0400);
        if (fd <= 0) {
            return EXIT_FAILURE;
        }
        close(fd);
    }

    return EXIT_SUCCESS;
}

int test_pipe_chown(void) {
    int fds[2] = { 0, 0 };

    if (pipe(fds)) {
        perror("pipe");
        return EXIT_FAILURE;
    }

    if (fchown(fds[0], 100, 200) || fchown(fds[1], 100, 200)) {
        perror("fchown");
        return EXIT_FAILURE;
    }
    close(fds[0]);
    close(fds[1]);

    return EXIT_SUCCESS;
}

int test_unlink(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Please specify at least a file name \n");
        return EXIT_FAILURE;
    }

    for (int i = 1; i != argc; i++) {
        if (unlink(argv[i]) < 0)
            return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int usr2_received = 0;

void usr2_handler(int v) {
    usr2_received = 1;
}

int test_set_signal_handler(int argc, char** argv) {

    struct sigaction act;
    act.sa_handler = usr2_handler;
    act.sa_flags = 0;
    sigemptyset(&act.sa_mask);
    if (sigaction(SIGUSR2, &act, NULL) < 0) {
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int test_wait_signal(int argc, char** argv) {
    while(!usr2_received) {
        sleep(1);
    }
    return EXIT_SUCCESS;
}

void *thread_exec(void *arg) {
    char **argv = (char **) arg;
    if (argv == NULL || argv[0] == NULL) {
        return NULL;
    }

    char *path_cpy = strdup(argv[0]);
    char *progname = basename(argv[0]);
    argv[0] = progname;

    execv(path_cpy, argv);
    return NULL;
}

int test_exec_in_pthread(int argc, char **argv) {
    if (argc <= 1) {
        return EXIT_FAILURE;
    }

    pthread_t thread;
    if (pthread_create(&thread, NULL, thread_exec, &argv[1]) < 0) {
        return EXIT_FAILURE;
    }
    pthread_join(thread, NULL);

    return EXIT_SUCCESS;
}

int test_sleep(int argc, char **argv) {
    if (argc != 2) {
        fprintf(stderr, "%s: Please pass a duration in seconds.\n", __FUNCTION__);
        return EXIT_FAILURE;
    }
    int duration = atoi(argv[1]);
    if (duration <= 0) {
        fprintf(stderr, "Please specify at a valid sleep duration\n");
    }
    sleep(duration);

    return EXIT_SUCCESS;
}

int test_slow_cat(int argc, char **argv) {
    if (argc != 3) {
        fprintf(stderr, "%s: Please pass a duration in seconds, and a path.\n", __FUNCTION__);
        return EXIT_FAILURE;
    }

    int duration = atoi(argv[1]);
    int fd = open(argv[2], O_RDONLY);

    if (duration <= 0) {
        fprintf(stderr, "Please specify at a valid sleep duration\n");
    }
    sleep(duration);

    close(fd);

    return EXIT_SUCCESS;
}

int test_slow_write(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "%s: Please pass a duration in seconds, a path, and a content.\n", __FUNCTION__);
        return EXIT_FAILURE;
    }

    int duration = atoi(argv[1]);
    int fd = open(argv[2], O_CREAT|O_WRONLY);

    if (duration <= 0) {
        fprintf(stderr, "Please specify at a valid sleep duration\n");
    }
    sleep(duration);

    write(fd, argv[3], strlen(argv[3]));

    close(fd);

    return EXIT_SUCCESS;
}

int test_memfd_create(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Please specify at least a file name \n");
        return EXIT_FAILURE;
    }

    for (int i = 1; i != argc; i++) {
        char *filename = argv[i];

        int fd = memfd_create(filename, 0);
        if (fd <= 0) {
            err(1, "%s failed", "memfd_create");
        }

        const char *script = "#!/bin/bash\necho Hello, world!\n";

        FILE *stream = fdopen(fd, "w");
        if (stream == NULL){
            err(1, "%s failed", "fdopen");
        }
        if (fputs(script, stream) == EOF){
            err(1, "%s failed", "fputs");
        }

        char * const argv[] = {filename, NULL};
        char * const envp[] = {NULL};
        fflush(stream);
        if (fexecve(fd, argv, envp) < 0){
            err(1, "%s failed", "fexecve");
        }

        fclose(stream);
    }

    return EXIT_SUCCESS;
}

int test_new_netns_exec(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "Please specify at least an executable path\n");
        return EXIT_FAILURE;
    }

    if (unshare(CLONE_NEWNET)) {
        perror("unshare");
        return EXIT_FAILURE;
    }

    execv(argv[1], argv + 1);
    fprintf(stderr, "execv failed: %s\n", argv[1]);
    return EXIT_FAILURE;
}

int test_network_flow_send_udp4(int argc, char **argv) {
    if (argc < 3) {
        fprintf(stderr, "Please specify the remote IP address and port\n");
        return EXIT_FAILURE;
    }

    int sockfd;
    struct sockaddr_in server_addr;
    const char *message = "DATA";

    // Create a DGRAM socket
    sockfd = socket(AF_INET, SOCK_DGRAM, 0);
    if (sockfd < 0) {
        fprintf(stderr, "Socket creation failed\n");
        return EXIT_FAILURE;
    }

    // Configure server address structure
    memset(&server_addr, 0, sizeof(server_addr));
    server_addr.sin_family = AF_INET;
    server_addr.sin_port = htons(atoi(argv[2]));
    server_addr.sin_addr.s_addr = inet_addr(argv[1]);

    // Send the message
    if (sendto(sockfd, message, strlen(message), 0, (struct sockaddr *)&server_addr, sizeof(server_addr)) < 0) {
        fprintf(stderr, "Failed to send data\n");
        close(sockfd);
        return EXIT_FAILURE;
    }

    printf("Message sent: %s\n", message);
    pid_t pid;

    // Get the process ID
    pid = getpid();
    printf("Process ID: %d\n", pid);

    // Close the socket
    close(sockfd);
    printf("Socket closed.\n");
    return EXIT_SUCCESS;
}

int test_chmod(int argc, char **argv) {
    if (argc != 3) {
        fprintf(stderr, "Please specify a file name and a mode\n");
        return EXIT_FAILURE;
    }

    const char *filename = argv[1];

    char *end;
    unsigned long mode = strtoul(argv[2], &end, 8);
    if (end == argv[2]) {
        fprintf(stderr, "Invalid mode: %s\n", argv[2]);
        return EXIT_FAILURE;
    } else if (*end != '\0') {
        fprintf(stderr, "Invalid mode: %s\n", argv[2]);
        return EXIT_FAILURE;
    } else if (errno == ERANGE && mode == ULONG_MAX) {
        fprintf(stderr, "Invalid mode: %s\n", argv[2]);
        return EXIT_FAILURE;
    } else if (mode > 0777) {
        fprintf(stderr, "Invalid mode: %s\n", argv[2]);
        return EXIT_FAILURE;
    }

    if (chmod(filename, mode) < 0) {
        perror("chmod");
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int test_chown(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "Please specify a file name, a user ID, and a group ID\n");
        return EXIT_FAILURE;
    }

    const char *filename = argv[1];

    char *end;
    unsigned long owner = strtoul(argv[2], &end, 10);
    if (end == argv[2]) {
        fprintf(stderr, "Invalid user ID: %s\n", argv[2]);
        return EXIT_FAILURE;
    } else if (*end != '\0') {
        fprintf(stderr, "Invalid user ID: %s\n", argv[2]);
        return EXIT_FAILURE;
    } else if (errno == ERANGE && owner == ULONG_MAX) {
        fprintf(stderr, "Invalid user ID: %s\n", argv[2]);
        return EXIT_FAILURE;
    } else if ((uid_t)owner != owner) {
        fprintf(stderr, "Invalid user ID: %s\n", argv[2]);
        return EXIT_FAILURE;
    }


    unsigned long group = strtoul(argv[3], &end, 10);
    if (end == argv[3]) {
        fprintf(stderr, "Invalid group ID: %s\n", argv[3]);
        return EXIT_FAILURE;
    } else if (*end != '\0') {
        fprintf(stderr, "Invalid group ID: %s\n", argv[3]);
        return EXIT_FAILURE;
    } else if (errno == ERANGE && group == ULONG_MAX) {
        fprintf(stderr, "Invalid group ID: %s\n", argv[3]);
        return EXIT_FAILURE;
    } else if ((gid_t)group != group) {
        fprintf(stderr, "Invalid user ID: %s\n", argv[2]);
        return EXIT_FAILURE;
    }

    if (chown(filename, (uid_t)owner, (gid_t)group) < 0) {
        perror("chown");
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int test_rename(int argc, char **argv) {
    if (argc != 3) {
        fprintf(stderr, "Please specify a source and a destination file name\n");
        return EXIT_FAILURE;
    }

    const char *oldpath = argv[1];
    const char *newpath = argv[2];

    if (rename(oldpath, newpath) < 0) {
        perror("rename");
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int test_utimes(int argc, char **argv) {
    if (argc != 2) {
        fprintf(stderr, "Please specify a file name and a time\n");
        return EXIT_FAILURE;
    }

    const char *filename = argv[1];

    struct timeval times[2] = {0, 0};
    if (utimes(filename, times) < 0) {
        perror("utimes");
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int test_link(int argc, char **argv) {
    if (argc != 3) {
        fprintf(stderr, "Please specify a source and a destination file name\n");
        return EXIT_FAILURE;
    }

    const char *oldpath = argv[1];
    const char *newpath = argv[2];

    if (link(oldpath, newpath) < 0) {
        perror("link");
        return EXIT_FAILURE;
    }

    return EXIT_SUCCESS;
}

int main(int argc, char **argv) {
    setbuf(stdout, NULL);

    if (argc <= 1) {
        fprintf(stderr, "Please pass a command\n");
        return EXIT_FAILURE;
    }

    for (int i = 1; i < argc; i++) {
        char *cmd = argv[i];

        int last_arg;
        for (last_arg = i + 1; last_arg < argc; last_arg++) {
            if (strcmp(argv[last_arg], ";") == 0) {
                argv[last_arg] = NULL;
                break;
            }
        }

        int sub_argc = last_arg - i;
        char **sub_argv = argv + i;
        int exit_code = 0;

        if (strcmp(cmd, "check") == 0) {
            exit_code = EXIT_SUCCESS;
        } else if (strcmp(cmd, "span-exec") == 0) {
            exit_code = span_exec(sub_argc, sub_argv);
        } else if (strcmp(cmd, "ptrace-traceme") == 0) {
            exit_code = ptrace_traceme();
        } else if (strcmp(cmd, "ptrace-attach") == 0) {
            exit_code = ptrace_attach();
        } else if (strcmp(cmd, "setrlimit-nofile") == 0) {
            exit_code = setrlimit_nofile();
        } else if (strcmp(cmd, "setrlimit-nproc") == 0) {
            exit_code = setrlimit_nproc();
        } else if (strcmp(cmd, "prlimit64-stack") == 0) {
            exit_code = prlimit64_stack();
        } else if (strcmp(cmd, "setrlimit-core") == 0) {
            exit_code = setrlimit_core();
        } else if (strcmp(cmd, "span-open") == 0) {
            exit_code = span_open(sub_argc, sub_argv);
        } else if (strcmp(cmd, "pipe-chown") == 0) {
            exit_code = test_pipe_chown();
        } else if (strcmp(cmd, "signal") == 0) {
            exit_code = test_signal(sub_argc, sub_argv);
        } else if (strcmp(cmd, "splice") == 0) {
            exit_code = test_splice();
        } else if (strcmp(cmd, "mkdirat") == 0) {
            exit_code = test_mkdirat(sub_argc, sub_argv);
        } else if (strcmp(cmd, "mkdirat-error") == 0) {
            exit_code = test_mkdirat_error(sub_argc, sub_argv);
        } else if (strcmp(cmd, "process-credentials") == 0) {
            exit_code = test_process_set(sub_argc, sub_argv);
        } else if (strcmp(cmd, "self-exec") == 0) {
            exit_code = self_exec(sub_argc, sub_argv);
        } else if (strcmp(cmd, "accept") == 0) {
            exit_code = test_accept(sub_argc, sub_argv);
        } else if (strcmp(cmd, "bind") == 0) {
            exit_code = test_bind(sub_argc, sub_argv);
        } else if (strcmp(cmd, "connect") == 0) {
            exit_code = test_connect(sub_argc, sub_argv);
        } else if (strcmp(cmd, "fork") == 0) {
            exit_code = test_forkexec(sub_argc, sub_argv);
        } else if (strcmp(cmd, "set-signal-handler") == 0) {
            exit_code = test_set_signal_handler(sub_argc, sub_argv);
        } else if (strcmp(cmd, "wait-signal") == 0) {
            exit_code = test_wait_signal(sub_argc, sub_argv);
        } else if (strcmp(cmd, "setregid") == 0) {
            exit_code = test_setregid(sub_argc, sub_argv);
        } else if (strcmp(cmd, "setreuid") == 0) {
            exit_code = test_setreuid(sub_argc, sub_argv);
        } else if (strcmp(cmd, "getchar") == 0) {
            exit_code = test_getchar(sub_argc, sub_argv);
        } else if (strcmp(cmd, "open") == 0) {
            exit_code = test_open(sub_argc, sub_argv);
        } else if (strcmp(cmd, "unlink") == 0) {
            exit_code = test_unlink(sub_argc, sub_argv);
        } else if (strcmp(cmd, "exec-in-pthread") == 0) {
            exit_code = test_exec_in_pthread(sub_argc, sub_argv);
        } else if (strcmp(cmd, "sleep") == 0) {
            exit_code = test_sleep(sub_argc, sub_argv);
        } else if (strcmp(cmd, "fileless") == 0) {
            exit_code = test_memfd_create(sub_argc, sub_argv);
        } else if (strcmp(cmd, "new_netns_exec") == 0) {
            exit_code = test_new_netns_exec(sub_argc, sub_argv);
        } else if (strcmp(cmd, "slow-cat") == 0) {
            exit_code = test_slow_cat(sub_argc, sub_argv);
        } else if (strcmp(cmd, "slow-write") == 0) {
            exit_code = test_slow_write(sub_argc, sub_argv);
        } else if (strcmp(cmd, "network_flow_send_udp4") == 0) {
            exit_code = test_network_flow_send_udp4(sub_argc, sub_argv);
        } else if (strcmp(cmd, "chmod") == 0) {
            exit_code = test_chmod(sub_argc, sub_argv);
        } else if (strcmp(cmd, "chown") == 0) {
            exit_code = test_chown(sub_argc, sub_argv);
        } else if (strcmp(cmd, "rename") == 0) {
            exit_code = test_rename(sub_argc, sub_argv);
        } else if (strcmp(cmd, "utimes") == 0) {
            exit_code = test_utimes(sub_argc, sub_argv);
        } else if (strcmp(cmd, "link") == 0) {
            exit_code = test_link(sub_argc, sub_argv);
        } else {
            fprintf(stderr, "Unknown command: %s\n", cmd);
            exit_code = EXIT_FAILURE;
        }

        if (exit_code != EXIT_SUCCESS) {
            fprintf(stderr, "Command `%s` failed: %d (errno: %s)\n", cmd, exit_code, strerror(errno));
            return exit_code;
        }

        i = last_arg;
    }

    return EXIT_SUCCESS;
}
