# pgsql-cluster-manager [![CircleCI](https://circleci.com/gh/gocardless/pgsql-cluster-manager.svg?style=svg&circle-token=38c8f4dc817216aa6a02b3bf67435fe2f1d72189)](https://circleci.com/gh/gocardless/pgsql-cluster-manager)

`pgsql-cluster-manager` extends a standard highly-available Postgres setup
(managed by [Corosync](http://corosync.github.io/) and
[Pacemaker](http://www.linux-ha.org/wiki/Pacemaker)) enabling its use in cloud
environments where using using floating IPs to denote the primary node is
difficult or impossible. In addition, `pgsql-cluster-manager` provides the
ability to run zero-downtime migrations of the Postgres primary with a simple
API trigger.

See [Playground](#playground) for how to start a Dockerised three node Postgres
cluster with `pgsql-cluster-manager`.

- [Overview](#overview)
    - [Playground](#playground)
    - [Node Roles](#node-roles)
        - [Postgres Nodes](#postgres-nodes)
        - [App Nodes](#app-nodes)
    - [Zero-Downtime Migrations](#zero-downtime-migrations)
- [Configuration](#configuration)
    - [Pacemaker](#pacemaker)
    - [PgBouncer](#pgbouncer)
- [Development](#development)
    - [CircleCI](#circleci)
    - [Releasing](#releasing)

## Overview

GoCardless runs a highly available Postgres cluster using
[Corosync](http://corosync.github.io/) and
[Pacemaker](http://www.linux-ha.org/wiki/Pacemaker). Corosync provides an
underlying quorum mechanism while pacemaker provides the ability to register
plugins that can manage arbitrary services, detecting and recovering from node
and service-level failures.

The typical Postgres setup with Corosync & Pacemaker uses a floating IP attached
to the Postgres primary node. Clients connect to this IP, and during failover
the IP is moved to the new primary. Managing portable IPs in Cloud providers
such as AWS and GCP is more difficult than a classic data center, and so we
built `pgsql-cluster-manager` to adapt our cluster for these environments.

`pgsql-cluster-manager` makes use of [etcd](https://github.com/coreos/etcd) to
store cluster configuration, which can then be used by clients to connect to the
appropriate node. We can view `pgsql-cluster-manager` as three distinct services
which each conceptually 'manage' different components:

- `cluster` extracts cluster state from pacemaker and pushes to etcd
- `proxy` ensures our Postgres proxy (PgBouncer) is reloaded with the current
  primary IP
- `migration` controls a zero-downtime migration flow

### Playground

We have created a Dockerised sandbox environment that boots a three node
Postgres cluster with the `pgsql-cluster-manager` services installed. We
strongly recommend playing around in this environment to develop an
understanding of how this setup works and to simulate failure situations
(network partitions, node crashes, etc).

**It also helps to have this playground running while reading through the README,
in order to try out the commands you see along the way.**

First install [Docker](https://docker.io/) and Golang >=1.9, then run:

```
# Clone into your GOPATH
$ git clone https://github.com/gocardless/pgsql-cluster-manager
$ cd pgsql-cluster-manager
$ go get github.com/gocardless/pgsql-cluster-manager/command
$ make build-linux

$ cd docker/postgres-member && ./start
Sending build context to Docker daemon 4.332 MB
Step 1/16 : FROM gocardless/pgsql-cluster-manager
...

root@pg01:/# crm_mon -Afr -1

Node Attributes:
* Node pg01:
    + Postgresql-data-status            : STREAMING|SYNC
    + Postgresql-status                 : HS:sync
    + master-Postgresql                 : 100
* Node pg02:
    + Postgresql-data-status            : STREAMING|POTENTIAL
    + Postgresql-status                 : HS:potential
    + master-Postgresql                 : -INFINITY
* Node pg03:
    + Postgresql-data-status            : LATEST
    + Postgresql-master-baseline        : 0000000002000090
    + Postgresql-status                 : PRI
    + master-Postgresql                 : 1000

root@pg01:/# ping pg03 -c1 | head -n1
PING pg03 (172.17.0.4) 56(84) bytes of data.

root@pg01:/# ETCDCTL_API=3 etcdctl get --prefix /
/postgres/master
172.17.0.4
```

The [start](docker/postgres-member/start) script will boot three Postgres nodes
with the appropriate configuration, and will start a full Postgres cluster. The
script (for convenience) will enter you into a docker shell in `pg01`.
Connecting to any of the other containers can be achieved with `docker exec -it
pg0X /bin/bash`.

### Node Roles

The `pgsql-cluster-manager` services are expected to run on two types of
machine: the nodes that are members of the Postgres cluster, and the machines
that will host applications which will connect to the cluster.

![Two node types, Postgres and App machines](res/node_roles.svg)

To explain how this setup works, we'll use an example of three machines (`pg01`,
`pg02`, `pg03`) to run the Postgres cluster and one machine (`app01`) to run our
client application. To match a typical production environment, let's imagine we
want to run a docker container on `app01` and have that container connect to our
Postgres cluster, while being resilient to Postgres failover.

It's worth noting that our playground configures only nodes of the Postgres
type, as this is sufficient to test out and play with the cluster. In production
you'd run app nodes so that applications can connect to the local PgBouncer,
which in turn knows how to route to the primary.

For playing around, it's totally fine to connect to one of the cluster nodes
PgBouncers directly from your host machine.

#### Postgres Nodes

In this hypothetical world we've provisioned our Postgres boxes with corosync,
pacemaker and Postgres, and additionally the following services:

- [PgBouncer](https://pgbouncer.github.io/) for connection pooling and proxying
  to the current primary
- [etcd](https://github.com/coreos/etcd) as a queryable store of cluster state,
  connecting to provide a three node etcd cluster

We then run the `cluster` service as a daemon, which will continually query
pacemaker to pull the current Postgres primary IP address and push this value to
etcd. Once we're pushing this value to etcd, we can use the `proxy` service to
subscribe to changes and update the local PgBouncer with the new value. We do
this by provisioning a PgBouncer [configuration template file](
docker/postgres-member/pgbouncer/pgbouncer.ini.template)
that looks like the following:

```
# /etc/pgbouncer/pgbouncer.ini.template

[databases]
postgres = host={{.Host}} pool_size=10
```

Whenever the `cluster` service pushes a new IP address to etcd, the `proxy`
service will render this template and replace any `{{.Host}}` placeholder with
the latest Postgres primary IP address, finally reloading PgBouncer to direct
connections at the new primary.

We can verify that `cluster` is pushing the IP address by using `etcdctl` to
inspect the contents of our etcd cluster. We should find the current Postgres
primary IP address has been pushed to the key we have configured for
`pgsql-cluster-manager`

```
root@pg01:/$ ETCDCTL_API=3 etcdctl get --prefix /
/postgres/master
172.17.0.2
```

#### App Nodes

We now have the Postgres nodes running PgBouncer proxies that live-update their
configuration to point connections to the latest Postgres primary. Our aim is
now to have app clients inside docker containers to connect to our Postgres
cluster without having to introduce routing decisions into the client code.

To do this, we install PgBouncer onto `app01` and bind to the host's private
interface. We then allow traffic from the docker network interface to the
private interface on the host, so that containers can communicate with the
PgBouncer on the host.

Finally we configure `app01`'s PgBouncer with a configuration template as we did
with the Postgres machines, and run the `proxy` service to continually update
PgBouncer to point at the latest primary. Containers then connect via the docker
host IP to PgBouncer, which will transparently direct connections to the correct
Postgres node.

```sh
root@app01:/$ cat <EOF >/etc/pgbouncer/pgbouncer.ini.template
[databases]
postgres = host={{.Host}}
EOF

root@app01:/$ service pgsql-cluster-manager-proxy start
pgsql-cluster-manager-proxy start/running, process 6997

root@app01:/$ service pgbouncer start
 * Starting PgBouncer pgbouncer
   ...done.

root@app01:/$ tail /var/log/pgsql-cluster-manager/proxy.log | grep HostChanger
{"handler":"*pgbouncer.HostChanger","key":"/master","level":"info","message":"Triggering handler with initial etcd key value","timestamp":"2017-12-03T17:49:03+0000","value":"172.17.0.2"}

root@app01:/$ tail /var/log/postgresql/pgbouncer.log | grep "RELOAD"
2017-12-03 17:49:03.167 16888 LOG RELOAD command issued

# Attempt to connect via the docker bridge IP
root@app01:/$ docker run -it --rm jbergknoff/postgresql-client postgresql://postgres@172.17.0.1:6432/postgres
Password:
psql (9.6.5, server 9.4.14)
Type "help" for help.

postgres=#
```

### Zero-Downtime Migrations

It's inevitable over the lifetime of a database cluster that machines will need
upgrading, and services restarting. It's not acceptable for such routine tasks
to require downtime, so `pgsql-cluster-manager` provides an API to trigger
migrations of the Postgres primary without disrupting database clients.

This API is served by the supervise `migration` service, which should be run on
all the Postgres nodes participating in the cluster. It's important to note that
this flow is only supported when all database clients are using PgBouncer
transaction pools in order to support pausing connections. Any clients that use
session pools will need to be turned off for the duration of the migration.

1. Acquire lock in etcd (ensuring only one migration takes place at a time)
2. Pause all PgBouncer pools on Postgres nodes
3. Instruct Pacemaker to perform migration of primary to sync node
4. Once the sync node is serving traffic as a primary, resume PgBouncer pools
5. Release etcd lock

As the primary moves machine, the supervise `cluster` service will push the new
IP address to etcd. The supervise `proxy` services running in the Postgres and
App nodes will detect this change and update PgBouncer to point at the new
primary IP, while the migration flow will detect this change in step (4) and
resume PgBouncer to allow queries to start once more.

```
root@pg01:/$ pgsql-cluster-manager --config-file /etc/pgsql-cluster-manager/config.toml migrate
INFO[0000] Loaded config                                 configFile=/etc/pgsql-cluster-manager/config.toml
INFO[0000] Health checking clients
INFO[0000] Acquiring etcd migration lock
INFO[0000] Pausing all clients
INFO[0000] Running crm resource migrate
INFO[0000] Watching for etcd key to update with master IP address  key=/master target=172.17.0.2
INFO[0006] Successfully migrated!                        master=pg01
INFO[0006] Running crm resource unmigrate
INFO[0007] Releasing etcd migration lock
```

This flow is subject to several timeouts that should be tuned to match your
pacemaker cluster settings. See `pgsql-cluster-manager migrate --help` for an
explanation of each timeout and how it affects the migration. This flow can be
run from anywhere that has access to the etcd and Postgres migration API.

The Postgres node that was originally the primary is now turned off, and won't
rejoin the cluster until the lockfile is removed. You can bring the node back
into the cluster by doing the following:

```
root@pg02:/$ rm /var/lib/postgresql/9.4/tmp/PGSQL.lock
root@pg02:/$ crm resource cleanup msPostgresql
```

## Configuration

We recommand configuring `pgsql-cluster-manager` using a TOML configuration
file. You can generate a sample configuration file with the default values for
each paramter by running the following:

```
$ pgsql-cluster-manager show-config >/etc/pgsql-cluster-manager/config.toml
```

### Pacemaker

The test environment is a good basis for configuring pacemaker with the pgsql
resource agent, and gives an example of cluster configuration that will
bootstrap a Postgres cluster.

We load pacemaker configuration in tests from the `configure_pacemaker` function
in [start-cluster.bash](docker/postgres-member/start-cluster.bash), though we
advise thinking carefully about what appropriate timeouts might be for your
setup.

The [pgsql](docker/postgres-member/resource_agents/pgsql) resource agent has
been modified to remove the concept of a primary floating IP. Anyone looking to
use this cluster without a floating IP will need to use the modified agent from
this repo, which renders the primary's actual IP directly into Postgres'
`recovery.conf` and reboots database replicas when the primary changes
(required, given Postgres cannot live reload `recovery.conf` changes).

### PgBouncer

We use [lib/pq](https://github.com/lib/pq) to connect to PgBouncer over the unix
socket. Unfortunately lib/pq has [issues](https://github.com/lib/pq/issues/475)
when first establishing a connection to PgBouncer as it attempts to set the
configuration parameters `extra_float_digits`, which PgBouncer doesn't
recognise, and therefore will reject the connection.

To avoid this, make sure all configuration templates include the following:

```
[pgbouncer]
...

# Connecting using the golang lib/pq wrapper requires that we ignore
# the 'extra_float_digits' startup parameter, otherwise PgBouncer will
# close the connection.
#
# https://github.com/lib/pq/issues/475
ignore_startup_parameters = extra_float_digits
```

## Development

### CircleCI

We build a custom Docker image for CircleCI builds that is hosted at
gocardless/pgsql-cluster-manager-circleci on Docker Hub. The Dockerfile lives at
`.circleci/Dockerfile`, and there is a make target to build and push the image.

To publish a new version of the Docker image, run:

```bash
make publish-circleci-dockerfile
```

### Releasing

We use [goreleaser](https://github.com/goreleaser/goreleaser) to create releases
for `pgsql-cluster-manager`. This enables us to effortlessly create new releases
with all associated artifacts to various destinations, such as GitHub and
homebrew taps.

To generate a new release, you must first tag the desired release commit and
then run `goreleaser` with a GitHub token for an account with write access to
this repo.

```sh
git tag v0.0.5 HEAD
GITHUB_TOKEN="..." goreleaser
```
