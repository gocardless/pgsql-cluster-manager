PROG=pgsql-cluster-manager
VERSION=$(shell git rev-parse --short HEAD)-dev
BUILD_COMMAND=go build -ldflags "-X github.com/gocardless/pgsql-cluster-manager/command.Version=$(VERSION)"

.PHONY: build build-integration test clean build-postgres-member-dockerfile publish-dockerfile publish-circleci-dockerfile

build:
	go generate ./...
	$(BUILD_COMMAND) -o $(PROG) main.go

build-linux:
	GOOS=linux GOARCH=amd64 $(BUILD_COMMAND) -o $(PROG).linux_amd64 main.go

build-integration:
	go test -tags integration -c github.com/gocardless/pgsql-cluster-manager/integration

test:
	go test ./...

export PGSQL_WORKSPACE=$(shell pwd)
test-integration: build-postgres-member-dockerfile
	[ -f $(PROG).linux_amd64 ] || (echo "Requires linux binary! Run `make build-linux` to build it" && exit 255)
	go test -tags integration -v github.com/gocardless/pgsql-cluster-manager/integration

clean:
	rm -rvf dist $(PROG) $(PROG).linux_amd64 *.test

build-postgres-member-dockerfile:
	docker build -t gocardless/postgres-member docker/postgres-member

publish-dockerfile:
	docker build -t gocardless/pgsql-cluster-manager-base . \
		&& docker push gocardless/pgsql-cluster-manager-base

publish-circleci-dockerfile:
	docker build -t gocardless/pgsql-cluster-manager-circleci .circleci \
		&& docker push gocardless/pgsql-cluster-manager-circleci
