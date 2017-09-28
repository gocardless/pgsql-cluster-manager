# Configures our base image for running tests in CircleCI
FROM gocardless/pgsql-cluster-manager

# Install docker client, required to use Docker in Circle
RUN curl "https://download.docker.com/linux/static/stable/x86_64/docker-17.06.2-ce.tgz" | tar xfvz - -C /tmp docker/docker && \
  mv -v /tmp/docker/docker /usr/bin/docker

# Run everything as circleci
ENV GOPATH=/home/circleci/go PATH=$PATH:/home/circleci/go/bin
RUN groupadd -g 1100 circleci && useradd -u 1100 -m -d /home/circleci -g circleci circleci

USER circleci
