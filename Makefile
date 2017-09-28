VERSION=0.0.1
PROG=pgsql-cluster-manager
PREFIX=/usr/local
BUILD_COMMAND=go build -ldflags "-X github.com/gocardless/pgsql-cluster-manager/command.Version=$(VERSION)"

.PHONY: build build-integration test $(PROG).linux_amd64 clean build-postgres-member-dockerfile publish-dockerfile publish-circleci-dockerfile

build:
	go generate ./...
	$(BUILD_COMMAND) -o $(PROG) main.go

build-integration:
	go test -tags integration -c github.com/gocardless/pgsql-cluster-manager/integration

test:
	go test ./...

export PGSQL_WORKSPACE=$(shell pwd)
test-integration: build-postgres-member-dockerfile
	[ -f *.deb ] || (echo "Requires deb package! Run `make deb` to build it." && exit 255)
	go test -tags integration -v github.com/gocardless/pgsql-cluster-manager/integration

deb: $(PROG).linux_amd64
	rm -fv *.deb
	bundle exec fpm -s dir -t $@ -n $(PROG) -v $(VERSION) \
		--architecture amd64 \
		--deb-no-default-config-files \
		--description "Orchestrator for Postgres clustering with corosync/pacemaker/etcd" \
		--maintainer "GoCardless Engineering <engineering@gocardless.com>" \
		$<=$(PREFIX)/bin/$(PROG)

$(PROG).linux_amd64:
	GOOS=linux GOARCH=amd64 $(BUILD_COMMAND) -o $(PROG).linux_amd64 *.go

clean:
	rm -vf $(PROG) $(PROG).linux_amd64 *.deb *.test

build-postgres-member-dockerfile:
	docker build -t gocardless/postgres-member docker/postgres-member

publish-dockerfile:
	docker build -t gocardless/pgsql-cluster-manager . \
		&& docker push gocardless/pgsql-cluster-manager

publish-circleci-dockerfile:
	docker build -t gocardless/pgsql-cluster-manager .circleci \
		&& docker push gocardless/pgsql-cluster-manager-circleci
