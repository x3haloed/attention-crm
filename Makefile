.PHONY: css build run test clean

GO ?= go

css:
	npm run build:css

build: css
	mkdir -p dist
	$(GO) build -o dist/attention ./cmd/attention

run: css
	$(GO) run ./cmd/attention

test:
	$(GO) test ./...

clean:
	rm -rf dist
