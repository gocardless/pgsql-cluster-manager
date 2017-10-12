#!/bin/bash
# We need to configure the cluster dynamically whenever we boot containers. This
# script allows us to bootstrap a cluster when provided with the three host=IP
# pairs that define the cluster nodes.

set -eu

[[ -t 1 ]] || exec >/var/log/start-cluster.log 2>&1

PG01="$1"
PG02="$2"
PG03="$3"

HOST="$(hostname -i | awk '{print $1}')"

function start_corosync() {
  echo "Generating corosync config"
  cat <<EOF > /etc/corosync/corosync.conf
totem {
        version: 2
        token: 3000
        token_retransmits_before_loss_const: 10
        join: 60
        consensus: 3600
        vsftype: none
        max_messages: 20
        clear_node_high_bit: yes
        secauth: on
        threads: 0

        rrp_mode: none
        interface {
                ringnumber: 0
                bindnetaddr: $(hostname -i)
                mcastport: 5405
        }
        transport: udpu
}

nodelist {
        node {
                ring0_addr: ${PG01}
                nodeid: 1
        }
        node {
                ring0_addr: ${PG02}
                nodeid: 2
        }
        node {
                ring0_addr: ${PG03}
                nodeid: 3
        }
}

amf {
        mode: disabled
}

quorum {
        provider: corosync_votequorum
}

aisexec {
        user:   root
        group:  root
}

logging {
        fileline: off
        to_stderr: yes
        to_logfile: yes
        to_syslog: no
        logfile: /var/log/corosync/corosync.log
        debug: off
        timestamp: on
        logger_subsys {
                subsys: AMF
                debug: off
                tags: enter|leave|trace1|trace2|trace3|trace4|trace6
        }
}
EOF

  chown root:root /etc/corosync/corosync.conf
  chmod 644 /etc/corosync/corosync.conf

  echo "Starting corosync/pacemaker"
  corosync # this starts corosync in the background
  service pacemaker start
}

function wait_for_quorum() {
  echo -n "Waiting for quorum..."
  until crm status | grep -q '3 Nodes configured'; do
      sleep 1 && printf "."
  done
  echo " done!"
}

function configure_corosync() {
  echo "Configuring corosync"
  cat <<EOF | crm configure
property stonith-enabled=false
property default-resource-stickiness=100
primitive Postgresql ocf:heartbeat:pgsql \
    params pgctl="/usr/lib/postgresql/9.4/bin/pg_ctl" psql="/usr/bin/psql" \
    pgdata="/var/lib/postgresql/9.4/main/" start_opt="-p 5432" rep_mode="sync" \
    node_list="pg01 pg02 pg03" primary_conninfo_opt="keepalives_idle=60 keepalives_interval=5 \
    keepalives_count=5" repuser="postgres" tmpdir="/var/lib/postgresql/9.4/tmp" \
    config="/etc/postgresql/9.4/main/postgresql.conf" \
    logfile="/var/log/postgresql/postgresql-crm.log" restore_command="exit 0" \
    op start timeout="60s" interval="0s" on-fail="restart" \
    op monitor timeout="60s" interval="2s" on-fail="restart" \
    op monitor timeout="60s" interval="1s" on-fail="restart" role="Master" \
    op promote timeout="60s" interval="0s" on-fail="restart" \
    op demote timeout="60s" interval="0s" on-fail="stop" \
    op stop timeout="60s" interval="0s" on-fail="block" \
    op notify timeout="60s" interval="0s"
ms msPostgresql Postgresql params master-max=1 master-node-max=1 clone-max=3 clone-node-max=1 notify=true
commit
end
EOF
}

function wait_for_roles() {
  echo "Waiting for master/sync/async..."
  while true; do
    echo "[$(date)] Polling..."
    (crm node list | grep 'LATEST') && \
      (crm node list | grep 'STREAMING|POTENTIAL') && \
      (crm node list | grep 'STREAMING|SYNC') && \
      return
    sleep 1
  done
}

function configure_dns() {
  cat <<EOF >>/etc/hosts
$PG01 pg01
$PG02 pg02
$PG03 pg03
EOF
}

function start_etcd() {
  echo "Starting etcd"
  /usr/bin/etcd \
    --name "$(hostname)" \
    --data-dir /tmp \
    --listen-peer-urls "http://$HOST:2380" \
    --initial-advertise-peer-urls "http://$HOST:2380" \
    --listen-client-urls http://0.0.0.0:2379 \
    --advertise-client-urls http://0.0.0.0:2379 \
    --initial-cluster "pg01=http://$PG01:2380,pg02=http://$PG02:2380,pg03=http://$PG03:2380" \
    --initial-cluster-token "some-randomness" \
    --initial-cluster-state new \
    >>/var/log/etcd.log 2>&1 &
  until etcdctl --endpoints http://127.0.0.1:2379 cluster-health 2>&1 /dev/null;
  do
    sleep 1
  done
}

function start_pgbouncer() {
  echo "Starting PGBouncer"
  service pgbouncer start
}

function start_cluster_manager() {
  echo "Installing pgsql-cluster-manager"
  dpkg -i /pgsql-cluster-manager/pgsql-cluster-manager*.deb
  cat <<EOF > /usr/local/bin/pgsql-cluster-manager.sh
#!/bin/bash

mkdir /var/log/pgsql

/usr/local/bin/pgsql-cluster-manager supervise cluster \
  --etcd-namespace /postgres \
  >>/var/log/pgsql/cluster.log 2>&1 &

/usr/local/bin/pgsql-cluster-manager supervise migration \
  >>/var/log/pgsql/migration.log 2>&1 &

sudo -u postgres \
  /usr/local/bin/pgsql-cluster-manager supervise proxy \
    --etcd-namespace /postgres \
    --pgbouncer-config-file /etc/pgbouncer/pgbouncer.ini \
    --pgbouncer-config-template-file /etc/pgbouncer/pgbouncer.ini.template \
    --postgres-master-etcd-key /master \
    --log-level debug \
    >>/var/log/pgsql/proxy.log 2>&1 &
EOF

  chmod a+x /usr/local/bin/pgsql-cluster-manager.sh
  /usr/local/bin/pgsql-cluster-manager.sh
}

start_corosync
wait_for_quorum

if [ "$(hostname -i)" == "$PG01" ]; then
  configure_corosync
fi

configure_dns

echo "Cluster is running"
