#ifndef __PID_TGID_H
#define __PID_TGID_H

#define GET_PID(x) ((x) >> 32)
#define GET_TGID(x) ((x) & 0xFFFFFFFF)

#endif // __PID_TGID_H
