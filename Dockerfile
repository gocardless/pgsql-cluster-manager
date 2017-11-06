# Generates an image that can be used as a base for dependencies
FROM ubuntu:trusty

ENV POSTGRESQL_VERSION=9.4 PGBOUNCER_VERSION=1.7.2-*
RUN set -x \
    && apt-get update \
    && apt-get install -y \
        software-properties-common \
        build-essential \
        curl \
        ruby-dev \
        wget \
        docker.io \
    && wget https://github.com/Yelp/dumb-init/releases/download/v1.2.0/dumb-init_1.2.0_amd64.deb \
    && dpkg -i dumb-init_*.deb && rm dumb-init_*.deb \
    && gem install bundler \
    && add-apt-repository ppa:gophers/archive \
    && echo "deb http://apt.postgresql.org/pub/repos/apt/ $(lsb_release -cs)-pgdg main\ndeb http://apt.postgresql.org/pub/repos/apt/ $(lsb_release -cs)-pgdg 9.4" > /etc/apt/sources.list.d/pgdg.list \
      && curl --silent -L https://www.postgresql.org/media/keys/ACCC4CF8.asc | apt-key add - \
    && apt-get update -y \
    && apt-get install -y \
        postgresql-"${POSTGRESQL_VERSION}" \
        pgbouncer="${PGBOUNCER_VERSION}" \
        corosync \
        pacemaker \
        golang-1.9-go \
    && ln -s /usr/lib/go-1.9/bin/go /usr/bin/go \
    && apt-get clean -y \
    && rm -rf /var/lib/apt/lists/*

ENV ETCD_VERSION=v3.2.6
RUN curl \
      -L https://storage.googleapis.com/etcd/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz \
      -o /tmp/etcd-linux-amd64.tar.gz && \
  mkdir /tmp/etcd && \
  tar xzvf /tmp/etcd-linux-amd64.tar.gz -C /tmp/etcd --strip-components=1 && \
  sudo mv -v /tmp/etcd/etcd /tmp/etcd/etcdctl /usr/bin/ && \
  rm -rfv /tmp/etcd-linux-amd64.tar.gz /tmp/etcd
