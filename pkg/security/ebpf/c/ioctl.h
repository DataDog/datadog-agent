#ifndef _IOCTL_H
#define _IOCTL_H

SEC("kprobe/do_vfs_ioctl")
int kprobe__do_vfs_ioctl(struct pt_regs *ctx) {

}

#endif