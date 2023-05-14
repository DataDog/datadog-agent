/* Source: include/net/tcp_states.h */

#ifndef _LINUX_TCP_STATES_H
#define _LINUX_TCP_STATES_H

enum
{
    TCP_ESTABLISHED = 1,
    TCP_SYN_SENT,
    TCP_SYN_RECV,
    TCP_FIN_WAIT1,
    TCP_FIN_WAIT2,
    TCP_TIME_WAIT,
    TCP_CLOSE,
    TCP_CLOSE_WAIT,
    TCP_LAST_ACK,
    TCP_LISTEN,
    TCP_CLOSING,
    TCP_NEW_SYN_RECV,

    TCP_MAX_STATES
};

#endif /* _LINUX_TCP_STATES_H */
