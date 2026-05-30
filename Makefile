CC ?= cc
CFLAGS ?= -O2 -Wall -Wextra -pedantic
LDFLAGS ?=
LDLIBS ?= -lsqlite3

.PHONY: all run test clean

all: glass-tower

install-deps:
	pnpm install
	pnpm approve-builds

glass-tower: install-deps server.c
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ server.c $(LDLIBS)
	node ./node_modules/minify/bin/minify.js public/app.js > public/app-minified.js

tests: tests.c server.c
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ tests.c $(LDLIBS)

test: tests
	./tests

run: glass-tower
	./glass-tower

clean:
	rm -f glass-tower tests
