.PHONY: all build clean stdio http shared tidy

# Build everything
all: build

build: shared stdio http

shared:
	cd shared && go build ./...

stdio:
	cd stdio-server && go build -o mcp-email-stdio

http:
	cd http-server && go build -o mcp-email-http

# Clean build artifacts
clean:
	rm -f stdio-server/mcp-email-stdio
	rm -f http-server/mcp-email-http

# Tidy all modules
tidy:
	cd shared && go mod tidy
	cd stdio-server && go mod tidy
	cd http-server && go mod tidy
