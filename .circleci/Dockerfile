# Generates an image with Golang and Ruby (for testing)
FROM circleci/ruby:2.4.1

RUN sudo apt-get update && \
	sudo apt-get install -y software-properties-common && \
	sudo apt-add-repository 'deb http://ppa.launchpad.net/longsleep/golang-backports/ubuntu xenial main' && \
	sudo apt-get update && sudo apt-get install golang-go=2:1.8~1ubuntu2~xenial

# We need postgres and pgbouncer libraries for testing
RUN sudo sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt/ $(lsb_release -cs)-pgdg main\ndeb http://apt.postgresql.org/pub/repos/apt/ $(lsb_release -cs)-pgdg 9.4" > /etc/apt/sources.list.d/pgdg.list' && \
  curl --silent https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo apt-key add - && \
  sudo apt-get update && sudo apt-get install -y pgbouncer=1.7.2-3.pgdg80+1

# Install etcd from Google's repositories
ENV ETCD_VERSION=v3.2.6
RUN curl \
      -L https://storage.googleapis.com/etcd/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz \
      -o /tmp/etcd-linux-amd64.tar.gz && \
  mkdir /tmp/etcd && \
  tar xzvf /tmp/etcd-linux-amd64.tar.gz -C /tmp/etcd --strip-components=1 && \
  sudo mv -v /tmp/etcd/etcd /tmp/etcd/etcdctl /usr/bin/ && \
  rm -rfv /tmp/etcd-linux-amd64.tar.gz /tmp/etcd

# Make go binaries available from our path
ENV PATH $PATH:/home/circleci/go/bin
RUN go get -u github.com/golang/dep/cmd/dep && \
      go get -u github.com/golang/lint/golint
