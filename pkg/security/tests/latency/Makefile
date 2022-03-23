name = bench_net_DNS
src = c/bench_net_DNS.c
obj = $(src:.c=.o)

%.o: %.c
	gcc -c -o $@ $<

all: $(obj)
	gcc -o $(name) $(obj)

clean:
	rm -f $(obj)

fclean: clean
	rm -f $(name)
