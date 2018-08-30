PROG=pgcm
SRC=cmd/pgcm/main.go
VERSION=$(shell git rev-parse --short HEAD)-dev
BUILD_COMMAND=go build -ldflags "-X github.com/gocardless/pgsql-cluster-manager/pkg/cmd.Version=$(VERSION)"

.PHONY: build build-integration test clean build-postgres-member-dockerfile publish-dockerfile publish-circleci-dockerfile

build:
	go generate ./...
	$(BUILD_COMMAND) -o $(PROG) $(SRC)

build-linux:
	GOOS=linux GOARCH=amd64 $(BUILD_COMMAND) -o $(PROG).linux_amd64 $(SRC)

build-integration:
	go test -tags integration -c github.com/gocardless/pgsql-cluster-manager/pkg/integration

test:
	go test ./...

export PGSQL_WORKSPACE=$(shell pwd)
test-integration: build-postgres-member-dockerfile
	[ -f $(PROG).linux_amd64 ] || (echo "Requires linux binary! Run `make build-linux` to build it" && exit 255)
	go test -tags integration -v github.com/gocardless/pgsql-cluster-manager/pkg/integration

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
