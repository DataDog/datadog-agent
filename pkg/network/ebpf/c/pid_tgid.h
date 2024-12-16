#ifndef __PID_TGID_H
#define __PID_TGID_H

#define GET_PID(x) ((x) & 0xFFFFFFFF)
#define GET_TGID(x) ((x) >> 32)

#endif // __PID_TGID_H
