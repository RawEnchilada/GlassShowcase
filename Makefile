CC ?= cc
CFLAGS ?= -O2 -Wall -Wextra -pedantic
LDFLAGS ?=
LDLIBS ?= -lsqlite3

.PHONY: all run test clean

all: glass-tower

glass-tower: server.c
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ server.c $(LDLIBS)

tests: tests.c server.c
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ tests.c $(LDLIBS)

test: tests
	./tests

run: glass-tower
	./glass-tower

clean:
	rm -f glass-tower tests
