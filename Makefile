.PHONY: css build run test clean release release-smoke

GO ?= go

css:
	npm run build:css

build: css
	mkdir -p dist
	$(GO) build -o dist/attention ./cmd/attention

release:
	./scripts/build_release.sh

release-smoke:
	./scripts/release_smoke.sh

run: css
	$(GO) run ./cmd/attention

test:
	$(GO) test ./...

clean:
	rm -rf dist
