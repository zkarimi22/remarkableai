GO ?= go
ZIG ?= zig
GOFLAGS ?=

.PHONY: all build test clean

all: build

build: bin/remarkableai-rm2 bin/type-text-rm2

bin/remarkableai-rm2: device/*.go device/go.mod device/ca-certificates.pem
	mkdir -p bin
	cd device && GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags='-s -w' -o ../$@

bin/type-text-rm2: tools/type_text.c
	mkdir -p bin
	$(ZIG) cc -target arm-linux-musleabihf -mcpu=cortex_a7 -static -O2 -Wall -Wextra -Werror $< -o $@

test:
	cd device && $(GO) test ./...
	python3 -m py_compile install.py ask.py

clean:
	rm -f bin/remarkableai-rm2 bin/type-text-rm2
