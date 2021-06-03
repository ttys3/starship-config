build:
	rm -rf bin
	gox -ldflags "-s -w" -osarch="linux/amd64 darwin/amd64 freebsd/amd64 windows/amd64 darwin/arm64 linux/arm64" -output="bin/{{.Dir}}.{{.OS}}_{{.Arch}}"
	ls -lhp bin

install:
	go install .

clean:
	rm -f os-icon
