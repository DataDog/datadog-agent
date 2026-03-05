/* ptwrap.c: a simple tool that runs a command in a pseudo-terminal */
/*
MIT License

Copyright (c) 2016-2019 WATANABE Yuki

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

#define _XOPEN_SOURCE 600
#define _DARWIN_C_SOURCE 1
#include <assert.h>
#include <errno.h>
#include <fcntl.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h> /* Not defined in X/Open */
#include <sys/select.h>
#include <sys/wait.h>
#include <termios.h>
#include <unistd.h>

static const char *program_name;

static void error_exit(const char *message) {
    fprintf(stderr, "%s: %s\n", program_name,  message);
    exit(EXIT_FAILURE);
}

static void errno_exit(const char *message) {
    fprintf(stderr, "%s: ", program_name);
    perror(message);
    exit(EXIT_FAILURE);
}

static int prepare_master_pseudo_terminal(void) {
    int fd = posix_openpt(O_RDWR | O_NOCTTY);
    if (fd < 0)
        errno_exit("cannot open master pseudo-terminal");
    if (fd <= STDERR_FILENO)
        error_exit("stdin/stdout/stderr are not open");

    if (grantpt(fd) < 0)
        errno_exit("pseudo-terminal permission not granted");
    if (unlockpt(fd) < 0)
        errno_exit("pseudo-terminal permission not unlocked");

    return fd;
}

static const char *slave_pseudo_terminal_name(int master_fd) {
    errno = 0; /* ptsname may not assign to errno, even if on error */
    const char *name = ptsname(master_fd);
    if (name == NULL)
        errno_exit("cannot name slave pseudo-terminal");
    return name;
}

static int open_noctty(const char *pathname) {
    int fd = open(pathname, O_RDWR | O_NOCTTY);
    if (fd < 0)
        errno_exit("cannot open slave pseudo-terminal");
    return fd;
}

enum state_T { INACTIVE, READING, WRITING, };
struct channel_T {
    int from_fd, to_fd;
    enum state_T state;
    char buffer[BUFSIZ];
    size_t buffer_position, buffer_length;
};

static void set_fd_set(
        struct channel_T *channel, fd_set *read_fds, fd_set *write_fds) {
    switch (channel->state) {
    case INACTIVE: break;
    case READING:  FD_SET(channel->from_fd, read_fds); break;
    case WRITING:  FD_SET(channel->to_fd, write_fds);  break;
    }
}

static void process_buffer(
        struct channel_T *channel, fd_set *read_fds, fd_set *write_fds) {
    ssize_t size;
    switch (channel->state) {
    case INACTIVE:
        break;
    case READING:
        if (!FD_ISSET(channel->from_fd, read_fds))
            break;
        channel->buffer_position = 0;
        size = read(channel->from_fd, channel->buffer, BUFSIZ);
        if (size <= 0) {
            channel->state = INACTIVE;
        } else {
            channel->state = WRITING;
            channel->buffer_length = size;
        }
        break;
    case WRITING:
        if (!FD_ISSET(channel->to_fd, write_fds))
            break;
        assert(channel->buffer_position < channel->buffer_length);
        size = write(channel->to_fd,
                &channel->buffer[channel->buffer_position],
                channel->buffer_length - channel->buffer_position);
        if (size < 0)
            break; /* ignore any error */
        channel->buffer_position += size;
        if (channel->buffer_position == channel->buffer_length)
            channel->state = READING;
        break;
    }
}

static void forward_all_io(int master_fd) {
    struct channel_T outgoing;
    outgoing.from_fd = master_fd;
    outgoing.to_fd = STDOUT_FILENO;
    outgoing.state = READING;

    /* Loop until all output from the slave is forwarded, so that we don't
     * miss any output. */
    while (outgoing.state != INACTIVE) {
        /* await next IO */
        fd_set read_fds, write_fds;
        FD_ZERO(&read_fds);
        FD_ZERO(&write_fds);
        set_fd_set(&outgoing, &read_fds, &write_fds);
        if (select(master_fd + 1, &read_fds, &write_fds, NULL, NULL) < 0)
            errno_exit("cannot find file descriptor to forward");

        /* read to or write from buffer */
        process_buffer(&outgoing, &read_fds, &write_fds);
    }
}

static int await_child(pid_t child_pid) {
    int wait_status;
    if (waitpid(child_pid, &wait_status, 0) != child_pid)
        errno_exit("cannot await child process");
    if (WIFEXITED(wait_status))
        return WEXITSTATUS(wait_status);
    if (WIFSIGNALED(wait_status))
        return WTERMSIG(wait_status) | 0x80;
    return EXIT_FAILURE;
}

static void become_session_leader(void) {
    if (setsid() < 0)
        errno_exit("cannot create new session");
}

static void prepare_slave_pseudo_terminal_fds(const char *slave_name) {
    /* How to become the controlling process of a slave pseudo-terminal is
     * implementation-dependent. We support two implementation schemes:
     * (1) A process automatically becomes the controlling process when it
     * first opens the terminal.
     * (2) A process needs to use the TIOCSCTTY ioctl system call.
     * There is a race condition in both schemes: an unrelated process could
     * become the controlling process before we do, in which case the slave is
     * not our controlling terminal and therefore we should abort. */

    if (close(STDIN_FILENO) < 0)
        errno_exit("cannot close old stdin");
    int slave_fd = open(slave_name, O_RDWR);
    if (slave_fd != STDIN_FILENO)
        errno_exit("cannot open slave pseudo-terminal at stdin");

    if (close(STDOUT_FILENO) < 0)
        errno_exit("cannot close old stdout");
    if (dup(slave_fd) != STDOUT_FILENO)
        errno_exit("cannot open slave pseudo-terminal at stdout");

    if (close(STDERR_FILENO) < 0)
        errno_exit("cannot close old stderr");
    if (dup(slave_fd) != STDERR_FILENO)
        errno_exit("cannot open slave pseudo-terminal at stderr");

#ifdef TIOCSCTTY
    ioctl(slave_fd, TIOCSCTTY, NULL);
#endif /* defined(TIOCSCTTY) */

    if (tcgetpgrp(slave_fd) != getpgrp())
        error_exit(
                "cannot become controlling process of slave pseudo-terminal");
}

static void exec_command(char *argv[]) {
    execvp(argv[0], argv);
    errno_exit(argv[0]);
}

int main(int argc, char *argv[]) {
    if (argc <= 0)
        exit(EXIT_FAILURE);
    program_name = argv[0];

    /* Don't use getopt, because we don't want glibc's reordering extension.
    if (getopt(argc, argv, "") != -1)
        exit(EXIT_FAILURE);
    */
    optind = 1;
    if (optind < argc && strcmp(argv[optind], "--") == 0)
        optind++;

    if (optind == argc)
        error_exit("operand missing");

    int master_fd = prepare_master_pseudo_terminal();
    const char *slave_name = slave_pseudo_terminal_name(master_fd);
    int slave_fd = open_noctty(slave_name);

    pid_t child_pid = fork();
    if (child_pid < 0)
        errno_exit("cannot spawn child process");
    if (child_pid > 0) {
        /* parent process */
        close(slave_fd);
        forward_all_io(master_fd);
        return await_child(child_pid);
    } else {
        /* child process */
        close(master_fd);
        become_session_leader();
        prepare_slave_pseudo_terminal_fds(slave_name);
        close(slave_fd);
        exec_command(&argv[optind]);
    }
}

/* vim: set et sw=4 sts=4 tw=79: */
