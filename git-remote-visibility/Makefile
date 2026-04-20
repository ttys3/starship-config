.PHONY: build install clean

build git-remote-visibility:
	go build -ldflags "-s -w"

install: build
	sudo install -m 0755 ./git-remote-visibility /usr/local/bin/git-remote-visibility

clean:
	rm -f git-remote-visibility
