#ifndef SSDEEP_H
#define SSDEEP_H

#include <stdint.h>
#include <stdio.h>

#define ROLLING_WINDOW  7
#define BLOCK_MIN       3u
#define NUM_BLOCKHASHES 31
#define SPAMSUM_LENGTH  64
#define HASH_INIT       0x27
#define SSDEEP_MAX_RESULT (SPAMSUM_LENGTH + SPAMSUM_LENGTH/2 + 20)

struct blockhash_state {
    unsigned char hashString[SPAMSUM_LENGTH];
    uint32_t blockSize;
    unsigned char h1, h2;
    unsigned char tail1, tail2;
    int hashLen;
};

struct ssdeep_state {
    uint32_t rh1, rh2, rh3, rn;
    unsigned char window[ROLLING_WINDOW];
    int iStart, iEnd;
    uint64_t totalSize;
    uint32_t bsizeMask;
    struct blockhash_state blocks[NUM_BLOCKHASHES];
};

int  ssdeep_init(struct ssdeep_state *s);
int  ssdeep_update(struct ssdeep_state *s, const unsigned char *data, int len);
int  ssdeep_digest(const struct ssdeep_state *s, char *result, int result_len);

#endif
