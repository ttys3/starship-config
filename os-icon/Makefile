os-icon:
	go build -ldflags "-s -w"

install: ./os-icon
	sudo install -m 0755 ./os-icon /usr/local/bin/os-icon

clean:
	rm -f os-icon
