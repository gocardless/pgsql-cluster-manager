FROM ubuntu:trusty

# Install basic tools required for installation
RUN apt-get update && apt-get install -y curl wget && \
  wget https://github.com/Yelp/dumb-init/releases/download/v1.2.0/dumb-init_1.2.0_amd64.deb && \
  dpkg -i dumb-init_*.deb && rm dumb-init_*.deb

# Install etcd from Google's repositories
ENV ETCD_VERSION=v3.2.6
RUN curl \
      -L "https://storage.googleapis.com/etcd/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz" \
      -o /tmp/etcd-linux-amd64.tar.gz && \
  mkdir /tmp/etcd && \
  tar xzvf /tmp/etcd-linux-amd64.tar.gz -C /tmp/etcd --strip-components=1 && \
  sudo mv -v /tmp/etcd/etcd /tmp/etcd/etcdctl /usr/bin/ && \
  rm -rfv /tmp/etcd-linux-amd64.tar.gz /tmp/etcd

# We need postgres and pgbouncer libraries
ENV POSTGRESQL_VERSION=9.4
ENV PGBOUNCER_VERSION=1.7.2-*
RUN sudo sh -c 'echo "deb http://apt.postgresql.org/pub/repos/apt/ $(lsb_release -cs)-pgdg main\ndeb http://apt.postgresql.org/pub/repos/apt/ $(lsb_release -cs)-pgdg 9.4" > /etc/apt/sources.list.d/pgdg.list' && \
  curl --silent https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo apt-key add - && \
  sudo apt-get update && \
  sudo apt-get install -y \
    postgresql-"${POSTGRESQL_VERSION}" \
    pgbouncer="${PGBOUNCER_VERSION}" \
    corosync \
    pacemaker

RUN rm -rf /var/lib/postgresql/9.4/main
ADD main.tar.gz /var/lib/postgresql/9.4/
COPY postgres/postgresql.conf postgres/pg_hba.conf /etc/postgresql/9.4/main/

COPY pgbouncer/userlist.txt pgbouncer/pgbouncer.ini.template /etc/pgbouncer/
RUN cp -v /etc/pgbouncer/pgbouncer.ini.template /etc/pgbouncer/pgbouncer.ini

COPY corosync/authkey /etc/corosync/
COPY resource_agents/pgsql /usr/lib/ocf/resource.d/heartbeat/

COPY start-cluster.bash /bin/start-cluster

EXPOSE 5432 6432 2379
ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["bash", "-c", "while :; do sleep 1; done"]
