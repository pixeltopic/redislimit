build-example:
	GOOS=linux GOARCH=amd64 go build -o ./bin/example ./cmd/example
build: build-example