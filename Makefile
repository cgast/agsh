.PHONY: build test clean docker-build docker-run

build:
	go build -o bin/agsh ./cmd/agsh

test:
	go test ./...

clean:
	rm -rf bin/

docker-build:
	docker build -f docker/Dockerfile -t agsh .

docker-run:
	docker-compose -f docker/docker-compose.yaml up
