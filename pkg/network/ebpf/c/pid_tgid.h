#ifndef __PID_TGID_H
#define __PID_TGID_H

/*
 * The following documentation is based on https://stackoverflow.com/a/9306150
 * Note on Process and Thread Identifiers:
 *
 * What users refer to as a "PID" is not quite the same as what the kernel sees.
 *
 * In the kernel:
 *  - Each thread has its own ID, called a PID (though it might be better termed a TID, or Thread ID).
 *  - Threads within the same process share a TGID (Thread Group ID), which is the PID of the first thread
 *    created when the process was initialized.
 *
 * When a process is created:
 *  - It starts as a single thread where the PID and TGID are the same.
 *
 * When a new thread is created:
 *  - It receives its own unique PID for independent scheduling by the kernel.
 *  - It inherits the TGID from the original (parent) thread, tying it to the same process.
 *
 * This separation allows the kernel to schedule threads independently while maintaining the process view
 * (TGID) when reporting information to users.
 *
 * Example Hierarchy of Threads:
 *
 *                           USER VIEW
 *                           vvvvvvvv
 *
 *              |
 * <-- PID 43 -->|<----------------- PID 42 ----------------->
 *              |                           |
 *              |      +---------+          |
 *              |      | process |          |
 *              |     _| pid=42  |_         |
 *         __(fork) _/ | tgid=42 | \_ (new thread) _
 *        /     |      +---------+          |       \
 * +---------+   |                           |    +---------+
 * | process |   |                           |    | process |
 * | pid=43  |   |                           |    | pid=44  |
 * | tgid=43 |   |                           |    | tgid=42 |
 * +---------+   |                           |    +---------+
 *              |                           |
 * <-- PID 43 -->|<--------- PID 42 -------->|<--- PID 44 --->
 *              |                           |
 *                         ^^^^^^^^
 *                         KERNEL VIEW
 */
#define GET_USER_MODE_PID(x) ((x) >> 32)
#define GET_KERNEL_THREAD_ID(x) ((x) & 0xFFFFFFFF)

#endif // __PID_TGID_H
