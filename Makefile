.PHONY: build install test

BINARY := synthwave-player-server
MODULE := github.com/Elive/synthwave-player

build:
	go build -ldflags="-s -w" -o $(BINARY) ./cmd/synthwave-player-server

install: build
	install -D -m 755 $(BINARY) $(DESTDIR)/usr/lib/synthwave-player/$(BINARY)
	install -D -m 755 tree/usr/bin/synthwave-player-server $(DESTDIR)/usr/bin/synthwave-player-server
	cp -r tree/usr/share/synthwave-player/public $(DESTDIR)/usr/share/synthwave-player/
	cp -r tree/usr/share/synthwave-player/templates $(DESTDIR)/usr/share/synthwave-player/
	rm -rf $(DESTDIR)/usr/share/synthwave-player/lib 2>/dev/null || true

test:
	go test ./...
