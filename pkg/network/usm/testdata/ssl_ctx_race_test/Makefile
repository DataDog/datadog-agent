# Build the ssl_ctx_race test helpers

CC = gcc
CFLAGS = -Wall -g -O2
LDFLAGS = -lssl -lcrypto

TARGETS = ssl_ctx_race ssl_ctx_race_v2

all: $(TARGETS)

ssl_ctx_race: ssl_ctx_race.c
	$(CC) $(CFLAGS) -o $@ $< $(LDFLAGS)

ssl_ctx_race_v2: ssl_ctx_race_v2.c
	$(CC) $(CFLAGS) -o $@ $< $(LDFLAGS)

clean:
	rm -f $(TARGETS)

.PHONY: all clean
