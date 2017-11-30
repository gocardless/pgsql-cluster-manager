# Configures our base image for running tests in CircleCI
FROM gocardless/pgsql-cluster-manager

# Install docker client, required to use Docker in Circle
RUN curl "https://download.docker.com/linux/static/stable/x86_64/docker-17.06.2-ce.tgz" | tar xfvz - -C /tmp docker/docker && \
  mv -v /tmp/docker/docker /usr/bin/docker

# We need goreleaser to build the go binaries, and fpm for debs
ENV GORELEASER_VERSION=v0.37.0
RUN curl -L "https://github.com/goreleaser/goreleaser/releases/download/${GORELEASER_VERSION}/goreleaser_Linux_x86_64.deb" --output /tmp/goreleaser.deb && \
  dpkg -i /tmp/goreleaser.deb && \
  rm -v /tmp/goreleaser.deb && \
  gem install fpm

# Run everything as circleci
ENV GOPATH=/home/circleci/go PATH=$PATH:/home/circleci/go/bin
RUN groupadd -g 1100 circleci && useradd -u 1100 -m -d /home/circleci -g circleci circleci

USER circleci
