#define _GNU_SOURCE
#include <sys/types.h>
#include <sys/socket.h>
#include <netdb.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <string.h>
#include <time.h>
#include <bsd/sys/time.h>
#include <errno.h>
#include <math.h>
#include <arpa/inet.h>

#define DNS_SERVER "127.0.0.1"

#ifndef timespecdiv
#define timespecdiv(a, d, res)                                          \
    do {                                                                \
        (res)->tv_sec = (a)->tv_sec / d;                                \
        (res)->tv_nsec = ((a)->tv_sec - ((res)->tv_sec * d)) * 1000000000L / d; \
        (res)->tv_nsec += (a)->tv_nsec / d;                             \
    } while (0)
#endif

/* benchmark counters */
static struct timespec _ts_start;
static struct timespec _ts_end;
static struct timespec _ts_res;
static struct timespec _ts_min = { .tv_sec = 999, .tv_nsec = 0 };
static struct timespec _ts_max = { .tv_sec = 0, .tv_nsec = 0 };
static struct timespec _ts_tot = { .tv_sec = 0, .tv_nsec = 0 };
static struct timespec _ts_avg;
static int _nb_runs = 0;
static int _to_skip = 0;

static inline void ts_start(void) {
    if (clock_gettime(CLOCK_MONOTONIC_RAW, &_ts_start) != 0) {
        perror("clock_gettime start");
        exit(EXIT_FAILURE);
    }
    return ;
}

static inline void ts_end(void) {
    if (_to_skip) {
        _to_skip--;
    } else {
        if (clock_gettime(CLOCK_MONOTONIC_RAW, &_ts_end) != 0) {
            perror("clock_gettime end: %s");
            exit(EXIT_FAILURE);
        }
        timespecsub(&_ts_end, &_ts_start, &_ts_res);
        timespecadd(&_ts_tot, &_ts_res, &_ts_tot);
        if (timespeccmp(&_ts_res, &_ts_min, <)) {
            _ts_min.tv_sec = _ts_res.tv_sec;
            _ts_min.tv_nsec = _ts_res.tv_nsec;
        }
        if (timespeccmp(&_ts_res, &_ts_max, >)) {
            _ts_max.tv_sec = _ts_res.tv_sec;
            _ts_max.tv_nsec = _ts_res.tv_nsec;
        }
        _nb_runs++;
    }
    return ;
}

static inline void ts_calcul_avg(void) {
    timespecdiv(&_ts_tot, _nb_runs, &_ts_avg);
    return ;
}

struct dns_header {
    unsigned short id;          // identification number

    unsigned char rd:1;         // recursion desired
    unsigned char tc:1;         // truncated message
    unsigned char aa:1;         // authoritive answer
    unsigned char opcode:4;     // purpose of message
    unsigned char qr:1;         // query/response flag

    unsigned char rcode:4;      // response code
    unsigned char cd:1;         // checking disabled
    unsigned char ad:1;         // authenticated data
    unsigned char z:1;          // its z! reserved
    unsigned char ra:1;         // recursion available

    unsigned short q_count;     // number of question entries
    unsigned short ans_count;   // number of answer entries
    unsigned short auth_count;  // number of authority entries
    unsigned short add_count;   // number of resource entries
};

struct question {
    unsigned short qtype;
    unsigned short qclass;
};

void host_to_dns(unsigned char *host, unsigned char *dns)
{
    int lock, i;

    for (lock = 0, i = 0; i < strlen((char *)host); i++) {
        if (host[i] == '.') {
            *dns++ = i - lock;
            for (; lock < i; lock++) {
                *dns++ = host[lock];
            }
            lock++;
        }
    }
    *dns++ = '\0';
}

int nslookup(unsigned char *host, int nb_req)
{
    unsigned char buf[65536];
    fd_set fds;

    /* init sockaddr to DNS server */
    struct sockaddr_in dest;
    dest.sin_family = AF_INET;
    dest.sin_port = htons(53);
    dest.sin_addr.s_addr = inet_addr(DNS_SERVER);

    /* set the DNS header */
    struct dns_header* dns = (struct dns_header *) &buf;
    memset(dns, 0, sizeof(*dns));
    dns->id = 42;
    dns->rd = 1; /* recursion desired */
    dns->q_count = htons(1); /* 1 question */

    /* set DNS query */
    unsigned char* qname = (unsigned char *) &buf + sizeof(*dns);
    host_to_dns(host, qname);
    struct question *qinfo = (struct question *) (qname + strlen((const char*)qname) + 1);
    qinfo->qtype = htons(1); /* ipv4 */
    qinfo->qclass = htons(1);

    /* calcul pkt len */
    unsigned int pktlen = sizeof(*dns) + (strlen((const char*)qname) + 1) + sizeof(*qinfo);

    /* main loop */
    for (int i = 0; i < nb_req; i++) {
        /* open socket */
        int s = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
        if (s  < 0) {
            perror("socket");
            return (-errno);
        }

        /* select until we can write */
        FD_ZERO(&fds);
        FD_SET(s, &fds);
        struct timeval tv; /* select timeout */
        tv.tv_sec = 5;
        tv.tv_usec = 0;
        int ret = select(s + 1, NULL, &fds, NULL, &tv);
        if (ret < 0) {
            perror("select");
            return (-errno);
        } else if (ret != 1) {
            fprintf(stderr, "Error: select returned %i\n", ret);
            return (-EIO);
        }

        /* send pkt */
        ts_start();
        int nbsent = sendto(s, (char *) buf, pktlen, 0, (struct sockaddr *) &dest, sizeof(dest));
        ts_end();

        /* check sendto errors */
        if (nbsent < 0) {
            perror("sendto");
            return (-errno);
        } else if (nbsent != pktlen) {
            fprintf(stderr, "Try to send %u oct, but only sent %i\n", pktlen, nbsent);
            return (-EIO);
        }

        /* close sock */
        close(s);
    }
    return (0);
}

void print_counter(const char* c, struct timespec* ts)
{
    if (ts->tv_sec)
        printf("%s: %lu.%09li sec\n", c, ts->tv_sec, ts->tv_nsec);
    else if (ts->tv_nsec < 1000)
        printf("%s: %li nsec\n", c, ts->tv_nsec);
    else
        printf("%s: %li usec\n", c, ts->tv_nsec / 1000);
    return ;
}

void print_stats(void)
{
    /* if we run more than 10 times, remove the min and max times */
    if (_nb_runs > 10) {
        timespecsub(&_ts_tot, &_ts_min, &_ts_tot);
        timespecsub(&_ts_tot, &_ts_max, &_ts_tot);
        printf("RESULT OF %i RUNS (minus the longest and the quickest):\n", _nb_runs);
        _nb_runs -= 2;
    } else printf("RESULT OF %i RUNS:\n", _nb_runs);

    ts_calcul_avg();
    print_counter("MIN", &_ts_min);
    print_counter("MAX", &_ts_max);
    print_counter("AVG", &_ts_avg);
    return ;
}

int main(int argc, char *argv[])
{
    int nb_req = 1;

    /* parse args */
    if (argc < 2 || argc > 4) {
        fprintf(stderr, "Usage: %s host [nb_req] [to_skip] )\n", argv[0]);
        exit(EXIT_FAILURE);
    }
    if (argc >= 3)
        nb_req = atoi(argv[2]);
    if (argc == 4)
        _to_skip = atoi(argv[3]);

    /* run DNS requests */
    char* host = NULL;
    int ret = asprintf(&host, "%s.", argv[1]);
    if (!host || ret < 0) {
        perror("asprintf");
        exit(errno);
    }
    ret = nslookup((unsigned char*)host, nb_req);
    free(host);

    /* print stats */
    print_stats();
    return (ret);
}
