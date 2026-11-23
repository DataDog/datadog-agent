#ifndef __DWARF_REGS_H__
#define __DWARF_REGS_H__

// DWARF_ARG_REGS(X) expands X once per Go register-ABI argument register,
// passing the DWARF register number. On amd64 DWARF_REGISTER() needs a literal
// number, so anything that reads a register chosen at runtime has to spell out
// all sixteen; this keeps the list in one place.
#define DWARF_ARG_REGS(X)                                                      \
  X(0) X(1) X(2) X(3) X(4) X(5) X(6) X(7)                                      \
  X(8) X(9) X(10) X(11) X(12) X(13) X(14) X(15)

#endif // __DWARF_REGS_H__
