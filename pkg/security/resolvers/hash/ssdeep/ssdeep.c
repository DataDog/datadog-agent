// ssdeep fuzzy hashing engine with precomputed sum_table.
// Algorithm matches glaslos/ssdeep v0.4.0 (MIT) and the official ssdeep.
// The lookup table replaces the per-byte ((h*0x93)^c)%64 with a single
// array access, avoiding multiply+XOR+modulo on every byte.

#include "ssdeep.h"
#include <string.h>
#include <unistd.h>
#include <errno.h>

#define HASH_PRIME 0x93

// Precomputed: sum_table[h][c] = ((h * 0x93) ^ c) % 64
// for h in [0,63], c in [0,63]. Only bottom 6 bits of input byte matter.
static const unsigned char sum_table[64][64] = {
#define ROW(h) { \
    ((h*HASH_PRIME)^0x00)%64,((h*HASH_PRIME)^0x01)%64,((h*HASH_PRIME)^0x02)%64,((h*HASH_PRIME)^0x03)%64, \
    ((h*HASH_PRIME)^0x04)%64,((h*HASH_PRIME)^0x05)%64,((h*HASH_PRIME)^0x06)%64,((h*HASH_PRIME)^0x07)%64, \
    ((h*HASH_PRIME)^0x08)%64,((h*HASH_PRIME)^0x09)%64,((h*HASH_PRIME)^0x0a)%64,((h*HASH_PRIME)^0x0b)%64, \
    ((h*HASH_PRIME)^0x0c)%64,((h*HASH_PRIME)^0x0d)%64,((h*HASH_PRIME)^0x0e)%64,((h*HASH_PRIME)^0x0f)%64, \
    ((h*HASH_PRIME)^0x10)%64,((h*HASH_PRIME)^0x11)%64,((h*HASH_PRIME)^0x12)%64,((h*HASH_PRIME)^0x13)%64, \
    ((h*HASH_PRIME)^0x14)%64,((h*HASH_PRIME)^0x15)%64,((h*HASH_PRIME)^0x16)%64,((h*HASH_PRIME)^0x17)%64, \
    ((h*HASH_PRIME)^0x18)%64,((h*HASH_PRIME)^0x19)%64,((h*HASH_PRIME)^0x1a)%64,((h*HASH_PRIME)^0x1b)%64, \
    ((h*HASH_PRIME)^0x1c)%64,((h*HASH_PRIME)^0x1d)%64,((h*HASH_PRIME)^0x1e)%64,((h*HASH_PRIME)^0x1f)%64, \
    ((h*HASH_PRIME)^0x20)%64,((h*HASH_PRIME)^0x21)%64,((h*HASH_PRIME)^0x22)%64,((h*HASH_PRIME)^0x23)%64, \
    ((h*HASH_PRIME)^0x24)%64,((h*HASH_PRIME)^0x25)%64,((h*HASH_PRIME)^0x26)%64,((h*HASH_PRIME)^0x27)%64, \
    ((h*HASH_PRIME)^0x28)%64,((h*HASH_PRIME)^0x29)%64,((h*HASH_PRIME)^0x2a)%64,((h*HASH_PRIME)^0x2b)%64, \
    ((h*HASH_PRIME)^0x2c)%64,((h*HASH_PRIME)^0x2d)%64,((h*HASH_PRIME)^0x2e)%64,((h*HASH_PRIME)^0x2f)%64, \
    ((h*HASH_PRIME)^0x30)%64,((h*HASH_PRIME)^0x31)%64,((h*HASH_PRIME)^0x32)%64,((h*HASH_PRIME)^0x33)%64, \
    ((h*HASH_PRIME)^0x34)%64,((h*HASH_PRIME)^0x35)%64,((h*HASH_PRIME)^0x36)%64,((h*HASH_PRIME)^0x37)%64, \
    ((h*HASH_PRIME)^0x38)%64,((h*HASH_PRIME)^0x39)%64,((h*HASH_PRIME)^0x3a)%64,((h*HASH_PRIME)^0x3b)%64, \
    ((h*HASH_PRIME)^0x3c)%64,((h*HASH_PRIME)^0x3d)%64,((h*HASH_PRIME)^0x3e)%64,((h*HASH_PRIME)^0x3f)%64  \
}
    ROW(0),ROW(1),ROW(2),ROW(3),ROW(4),ROW(5),ROW(6),ROW(7),
    ROW(8),ROW(9),ROW(10),ROW(11),ROW(12),ROW(13),ROW(14),ROW(15),
    ROW(16),ROW(17),ROW(18),ROW(19),ROW(20),ROW(21),ROW(22),ROW(23),
    ROW(24),ROW(25),ROW(26),ROW(27),ROW(28),ROW(29),ROW(30),ROW(31),
    ROW(32),ROW(33),ROW(34),ROW(35),ROW(36),ROW(37),ROW(38),ROW(39),
    ROW(40),ROW(41),ROW(42),ROW(43),ROW(44),ROW(45),ROW(46),ROW(47),
    ROW(48),ROW(49),ROW(50),ROW(51),ROW(52),ROW(53),ROW(54),ROW(55),
    ROW(56),ROW(57),ROW(58),ROW(59),ROW(60),ROW(61),ROW(62),ROW(63)
#undef ROW
};

static inline unsigned char sum_hash(unsigned char c, unsigned char h) {
    return sum_table[h][c & 0x3f];
}

int ssdeep_init(struct ssdeep_state *s) {
    if (s == NULL)
        return -1;
    memset(s, 0, sizeof(*s));
    s->iEnd = 1;
    for (int i = 0; i < NUM_BLOCKHASHES; i++) {
        s->blocks[i].blockSize = BLOCK_MIN << i;
        s->blocks[i].h1 = HASH_INIT;
        s->blocks[i].h2 = HASH_INIT;
        s->blocks[i].hashLen = 0;
    }
    return 0;
}

static inline int is_start_block_full(const struct ssdeep_state *s) {
    return s->totalSize > (uint64_t)s->blocks[s->iStart].blockSize * SPAMSUM_LENGTH
        && s->blocks[s->iStart + 1].hashLen >= SPAMSUM_LENGTH / 2;
}

static void ssdeep_engine(struct ssdeep_state *s, const unsigned char *data, int len) {
    uint32_t rh1 = s->rh1, rh2 = s->rh2, rh3 = s->rh3, rn = s->rn;

    s->totalSize += (uint64_t)len;

    for (int di = 0; di < len; di++) {
        unsigned char b = data[di];

        for (int i = s->iStart; i < s->iEnd; i++) {
            s->blocks[i].h1 = sum_hash(b, s->blocks[i].h1);
            s->blocks[i].h2 = sum_hash(b, s->blocks[i].h2);
        }

        // Rolling hash (Adler-like)
        rh2 -= rh1;
        rh2 += ROLLING_WINDOW * (uint32_t)b;
        rh1 += (uint32_t)b;
        rh1 -= (uint32_t)s->window[rn];
        s->window[rn] = b;
        if (++rn == ROLLING_WINDOW)
            rn = 0;
        rh3 = (rh3 << 5) ^ (uint32_t)b;

        uint32_t rh = rh1 + rh2 + rh3;
        if (rh == 0xFFFFFFFFu)
            continue;
        if (((rh + 1) / BLOCK_MIN) & s->bsizeMask)
            continue;
        if ((rh + 1) % BLOCK_MIN)
            continue;

        for (int i = s->iStart; i < s->iEnd; i++) {
            struct blockhash_state *block = &s->blocks[i];
            if (rh % block->blockSize != (block->blockSize - 1))
                continue;

            if (block->hashLen == 0) {
                if (s->iEnd <= NUM_BLOCKHASHES - 1) {
                    struct blockhash_state *old = &s->blocks[s->iEnd - 1];
                    struct blockhash_state *newb = &s->blocks[s->iEnd];
                    newb->h1 = old->h1;
                    newb->h2 = old->h2;
                    s->iEnd++;
                }
            }
            block->tail1 = block->h1;
            block->tail2 = block->h2;
            if (block->hashLen < SPAMSUM_LENGTH - 1) {
                block->hashString[block->hashLen++] = block->tail1;
                block->tail1 = 0;
                block->h1 = HASH_INIT;
                if (block->hashLen < SPAMSUM_LENGTH / 2) {
                    block->h2 = HASH_INIT;
                    block->tail2 = 0;
                }
            } else if (is_start_block_full(s)) {
                s->iStart++;
                s->bsizeMask = (s->bsizeMask << 1) + 1;
            }
        }
    }

    s->rh1 = rh1;
    s->rh2 = rh2;
    s->rh3 = rh3;
    s->rn = rn;
}

int ssdeep_update(struct ssdeep_state *s, const unsigned char *data, int len) {
    if (s == NULL || data == NULL || len < 0)
        return -1;
    ssdeep_engine(s, data, len);
    return 0;
}

int ssdeep_digest(const struct ssdeep_state *s, char *result, int result_len) {
    if (s == NULL || result == NULL || result_len < SSDEEP_MAX_RESULT)
        return -1;

    int i = s->iStart;
    while (i < NUM_BLOCKHASHES - 1 &&
           (uint64_t)((uint32_t)BLOCK_MIN << i) * SPAMSUM_LENGTH < s->totalSize)
        i++;

    if (i >= s->iEnd)
        i = s->iEnd - 1;
    while (i > s->iStart && s->blocks[i].hashLen < SPAMSUM_LENGTH / 2)
        i--;

    unsigned char buf1[SPAMSUM_LENGTH + 1];
    unsigned char buf2[SPAMSUM_LENGTH + 1];
    int len1, len2;

    memcpy(buf1, s->blocks[i].hashString, s->blocks[i].hashLen);
    len1 = s->blocks[i].hashLen;

    if (i >= s->iEnd - 1) {
        memcpy(buf2, s->blocks[i].hashString, s->blocks[i].hashLen);
        len2 = s->blocks[i].hashLen;
    } else {
        memcpy(buf2, s->blocks[i + 1].hashString, s->blocks[i + 1].hashLen);
        len2 = s->blocks[i + 1].hashLen;
    }

    if (len2 > SPAMSUM_LENGTH / 2 - 1)
        len2 = SPAMSUM_LENGTH / 2 - 1;

    uint32_t rh = s->rh1 + s->rh2 + s->rh3;
    if (rh != 0) {
        buf1[len1++] = s->blocks[i].h1;
        if (i < s->iEnd - 1)
            buf2[len2++] = s->blocks[i + 1].h2;
        else
            buf2[len2++] = s->blocks[i].h2;
    } else {
        if (len1 == SPAMSUM_LENGTH - 1 && s->blocks[i].tail1 != 0)
            buf1[len1++] = s->blocks[i].tail1;
        if (i < s->iEnd - 1) {
            if (s->blocks[i + 1].tail2 != 0)
                buf2[len2++] = s->blocks[i + 1].tail2;
        } else {
            if (s->blocks[i].tail2 != 0)
                buf2[len2++] = s->blocks[i].tail2;
        }
    }

    static const char b64[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    for (int j = 0; j < len1; j++)
        buf1[j] = b64[buf1[j]];
    for (int j = 0; j < len2; j++)
        buf2[j] = b64[buf2[j]];
    buf1[len1] = '\0';
    buf2[len2] = '\0';

    int n = snprintf(result, result_len, "%u:%s:%s",
                     s->blocks[i].blockSize, buf1, buf2);
    if (n < 0)
        return -1;
    if (n >= result_len)
        n = result_len - 1;
    return n;
}

#define READ_BUF_SIZE 32768

int ssdeep_hash_fd(int fd, int64_t max_size, char *result, int result_len) {
    if (fd < 0 || result == NULL || result_len < SSDEEP_MAX_RESULT)
        return SSDEEP_ERR_ARGS;

    struct ssdeep_state state;
    ssdeep_init(&state);

    unsigned char buf[READ_BUF_SIZE];
    int64_t total = 0;

    for (;;) {
        ssize_t n = read(fd, buf, READ_BUF_SIZE);
        if (n < 0) {
            if (errno == EINTR)
                continue;
            return SSDEEP_ERR_READ;
        }
        if (n == 0)
            break;

        total += n;
        if (max_size > 0 && total > max_size)
            return SSDEEP_ERR_TOO_BIG;

        ssdeep_engine(&state, buf, (int)n);
    }

    if (total < SSDEEP_MIN_FILE_SIZE)
        return SSDEEP_ERR_TOO_SMALL;

    int digest_len = ssdeep_digest(&state, result, result_len);
    if (digest_len <= 0)
        return SSDEEP_ERR_DIGEST;

    return digest_len;
}
