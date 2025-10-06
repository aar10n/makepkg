.PHONY: build build-linux clean install docker-build docker-test docker-shell

build:
	mkdir -p bin
	go build -o bin/makepkg

build-linux:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/makepkg-linux

clean:
	rm -rf bin/

install: build
	@if [ -z "$(BINDIR)" ]; then \
		echo "Please set BINDIR to the installation directory (e.g., /usr/local/bin)"; \
		exit 1; \
	fi
	@if [ -z "$(MANDIR)" ]; then \
  		echo "Please set MANDIR to the installation directory (e.g., /usr/local/bin)"; \
  		exit 1; \
	fi
	install -m 755 bin/makepkg $(BINDIR)/makepkg
	mkdir -p $(MANDIR)/man1
	install -m 644 makepkg.1 $(MANDIR)/man1/makepkg.1

# Docker targets for testing on Linux x86_64
docker-build: build-linux
	docker build -f Dockerfile.test -t makepkg-test:latest .

docker-test: docker-build
	docker run --rm makepkg-test:latest \
		./makepkg --sysroot /tmp/test-sysroot --builddir /tmp/build -j 2 -f examples/packages.test.yaml -t examples/toolchain.test.yaml

docker-shell: docker-build
	docker run --rm -it -v $(PWD)/build:/workspace/build makepkg-test:latest /bin/bash
