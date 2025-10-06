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
	install -D -m 755 bin/makepkg $(DESTDIR)/usr/local/bin/makepkg

# Docker targets for testing on Linux x86_64
docker-build: build-linux
	docker build -f Dockerfile.test -t makepkg-test:latest .

docker-test: docker-build
	docker run --rm makepkg-test:latest \
		./makepkg --sysroot /tmp/test-sysroot --builddir /tmp/build -j 2 -f examples/packages.test.yaml -t examples/toolchain.test.yaml

docker-shell: docker-build
	docker run --rm -it -v $(PWD)/build:/workspace/build makepkg-test:latest /bin/bash
