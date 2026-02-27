# Build the bio_leak test helper

CC = gcc
CFLAGS = -Wall -g -O2
LDFLAGS = -lssl -lcrypto

TARGET = bio_leak

all: $(TARGET)

$(TARGET): bio_leak.c
	$(CC) $(CFLAGS) -o $@ $< $(LDFLAGS)

clean:
	rm -f $(TARGET)

.PHONY: all clean

