# Build the ssl_threadpool test helper

CC = gcc
CFLAGS = -Wall -g -O2
LDFLAGS = -lssl -lcrypto -lpthread

TARGET = ssl_threadpool

all: $(TARGET)

$(TARGET): ssl_threadpool.c
	$(CC) $(CFLAGS) -o $@ $< $(LDFLAGS)

clean:
	rm -f $(TARGET)

.PHONY: all clean
