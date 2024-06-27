/*
    These are types defined in the Go runtime.
    We have to redefine them here. Their 
    definitions are not a stable ABI. Care
    needs to be taken to make sure they're the
    same between different versions of Go.
*/

struct stack {
  uintptr_t lo;
  uintptr_t hi;
};

struct gobuf {
  uintptr_t sp;
  uintptr_t pc;
  uintptr_t g;
  uintptr_t ctxt;
  uintptr_t ret;
  uintptr_t lr;
  uintptr_t bp;
};

struct g {
  struct stack stack;
  uintptr_t stackguard0;
  uintptr_t stackguard1;
  uintptr_t _panic;
  uintptr_t _defer;
  uintptr_t m;
  struct gobuf sched;
  uintptr_t syscallsp;
  uintptr_t syscallpc;
  uintptr_t stktopsp;
  uintptr_t param;
  uint32_t atomicstatus;
  uint32_t stackLock;
  uint64_t goid;
};
