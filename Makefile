APP=svn_to_git
MODULE=github.com/dasmlab/svn-2-git

.PHONY: build run tidy clean

build:
	CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/$(APP) ./cmd/svn_to_git

run:
	./bin/$(APP) --help || true

tidy:
	go mod tidy

clean:
	rm -rf bin


