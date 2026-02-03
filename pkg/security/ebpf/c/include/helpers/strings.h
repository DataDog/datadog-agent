#ifndef _HELPERS_STRINGS_H
#define _HELPERS_STRINGS_H

void __attribute__((always_inline)) clean_str_trailing_zeros(char *s, int string_size, int array_size) {
    int nul = 15;

    if (s[0] == 0) nul = 0;
    else if (s[1] == 0) nul = 1;
    else if (s[2] == 0) nul = 2;
    else if (s[3] == 0) nul = 3;
    else if (s[4] == 0) nul = 4;
    else if (s[5] == 0) nul = 5;
    else if (s[6] == 0) nul = 6;
    else if (s[7] == 0) nul = 7;
    else if (s[8] == 0) nul = 8;
    else if (s[9] == 0) nul = 9;
    else if (s[10] == 0) nul = 10;
    else if (s[11] == 0) nul = 11;
    else if (s[12] == 0) nul = 12;
    else if (s[13] == 0) nul = 13;
    else if (s[14] == 0) nul = 14;

    switch (nul) {
        case 0:  s[1]  = 0;
        case 1:  s[2]  = 0;
        case 2:  s[3]  = 0;
        case 3:  s[4]  = 0;
        case 4:  s[5]  = 0;
        case 5:  s[6]  = 0;
        case 6:  s[7]  = 0;
        case 7:  s[8]  = 0;
        case 8:  s[9]  = 0;
        case 9:  s[10] = 0;
        case 10: s[11] = 0;
        case 11: s[12] = 0;
        case 12: s[13] = 0;
        case 13: s[14] = 0;
        case 14: s[15] = 0;
    default:
        break;
    }

    s[array_size - 1] = 0;
}

#endif
