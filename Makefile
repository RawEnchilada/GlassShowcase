
.PHONY: all run test clean minify
 
all: glass-tower
 
glass-tower:
	go build -ldflags="-s -w" -o $@ .
 
minify:
	pnpm install
	pnpm approve-builds
	node ./node_modules/minify/bin/minify.js public/app.js > public/app-minified.js
 
test:
	go test ./...
 
run: glass-tower
	./glass-tower
 
clean:
	rm -f glass-tower
 
