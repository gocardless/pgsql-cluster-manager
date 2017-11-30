PROG=pgsql-cluster-manager
BUILD_COMMAND=go build -ldflags "-X github.com/gocardless/pgsql-cluster-manager/command.Version=$(git rev-parse --short HEAD)-dev"

.PHONY: build build-integration test clean build-postgres-member-dockerfile publish-dockerfile publish-circleci-dockerfile

build:
	go generate ./...
	go build -o $(PROG) main.go

build-integration:
	go test -tags integration -c github.com/gocardless/pgsql-cluster-manager/integration

test:
	go test ./...

export PGSQL_WORKSPACE=$(shell pwd)
test-integration: build-postgres-member-dockerfile
	[ -f dist/*.deb ] || (echo "Requires deb package! Run `goreleaser` to build it." && exit 255)
	go test -tags integration -v github.com/gocardless/pgsql-cluster-manager/integration

clean:
	rm -rvf dist $(PROG) *.test

build-postgres-member-dockerfile:
	docker build -t gocardless/postgres-member docker/postgres-member

publish-dockerfile:
	docker build -t gocardless/pgsql-cluster-manager . \
		&& docker push gocardless/pgsql-cluster-manager

publish-circleci-dockerfile:
	docker build -t gocardless/pgsql-cluster-manager-circleci .circleci \
		&& docker push gocardless/pgsql-cluster-manager-circleci
