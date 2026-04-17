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
#define SSDEEP_MIN_FILE_SIZE 4096

// Return codes for ssdeep_hash_fd
#define SSDEEP_ERR_ARGS      (-1)
#define SSDEEP_ERR_READ      (-2)
#define SSDEEP_ERR_TOO_BIG   (-3)
#define SSDEEP_ERR_TOO_SMALL (-4)
#define SSDEEP_ERR_DIGEST    (-5)

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

// Single-call: read from fd, hash, produce digest. One CGO crossing.
// Returns digest length on success, or a negative SSDEEP_ERR_* code.
int  ssdeep_hash_fd(int fd, int64_t max_size, char *result, int result_len);

#endif
