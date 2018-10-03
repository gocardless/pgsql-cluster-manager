#!/bin/bash
# We need to configure the cluster dynamically whenever we boot containers. This
# script allows us to bootstrap a cluster when provided with the three host=IP
# pairs that define the cluster nodes.

set -eu

[[ -t 1 ]] || exec >/var/log/start-cluster.log 2>&1

function log() {
  echo "[$(date)] $1"
}

# container_ip <id>
function container_ip() {
  docker inspect -f '{{ .NetworkSettings.IPAddress }}' "$1"
}

PG01="$1"; PG01_IP="$(container_ip "$PG01")"
PG02="$2"; PG02_IP="$(container_ip "$PG02")"
PG03="$3"; PG03_IP="$(container_ip "$PG03")"

HOST="$(hostname -i | awk '{print $1}')"

function start_corosync() {
  log "Generating corosync config"
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
                ring0_addr: ${PG01_IP}
                nodeid: 1
        }
        node {
                ring0_addr: ${PG02_IP}
                nodeid: 2
        }
        node {
                ring0_addr: ${PG03_IP}
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

  log "Starting corosync/pacemaker"
  corosync # this starts corosync in the background
  service pacemaker start
}

function wait_for_quorum() {
  log -n "Waiting for quorum..."
  until crm status | grep -q '3 Nodes configured'; do
      sleep 1 && printf "."
  done
  log " done!"
}

function configure_pacemaker() {
  log "Configuring pacemaker"
  cat <<EOF | crm configure
property stonith-enabled=true
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
primitive shoot-pg01 stonith:external/docker params server_id="$PG01" server_name="pg01"
location fence_pg01 shoot-pg01 -inf: pg01
primitive shoot-pg02 stonith:external/docker params server_id="$PG02" server_name="pg02"
location fence_pg02 shoot-pg02 -inf: pg02
primitive shoot-pg03 stonith:external/docker params server_id="$PG03" server_name="pg03"
location fence_pg03 shoot-pg03 -inf: pg03
commit
end
EOF
}

function wait_for_roles() {
  log "Waiting for master/sync/async..."
  while true; do
    log "Polling..."
    (crm node list | grep 'LATEST') && \
      (crm node list | grep 'STREAMING|POTENTIAL') && \
      (crm node list | grep 'STREAMING|SYNC') && \
      return
    sleep 1
  done
}

function configure_dns() {
  cat <<EOF >>/etc/hosts
$PG01_IP pg01
$PG02_IP pg02
$PG03_IP pg03
EOF
}

function start_etcd() {
  log "Starting etcd"
  /usr/bin/etcd \
    --name "$(hostname)" \
    --data-dir /tmp \
    --listen-peer-urls "http://$HOST:2380" \
    --initial-advertise-peer-urls "http://$HOST:2380" \
    --listen-client-urls http://0.0.0.0:2379 \
    --advertise-client-urls http://0.0.0.0:2379 \
    --initial-cluster "pg01=http://$PG01_IP:2380,pg02=http://$PG02_IP:2380,pg03=http://$PG03_IP:2380" \
    --initial-cluster-token "some-randomness" \
    --initial-cluster-state new \
    >>/var/log/etcd.log 2>&1 &
  until etcdctl --endpoints http://127.0.0.1:2379 cluster-health 2>&1 /dev/null;
  do
    sleep 1
  done
}

function start_pgbouncer() {
  log "Starting PgBouncer"
  service pgbouncer start
}

function start_cluster_manager() {
  log "Installing pgsql-cluster-manager"
  cp -v /pgsql-cluster-manager/bin/pgcm.linux_amd64 /usr/local/bin/pgcm
  cat <<EOF > /usr/local/bin/pgsql-cluster-manager.sh
#!/bin/bash

mkdir /var/log/pgsql-cluster-manager

/usr/local/bin/pgcm supervise \
  --config-file /etc/pgsql-cluster-manager/config.toml \
  >>/var/log/pgsql-cluster-manager/supervise.log 2>&1 &

sudo -u postgres \
  /usr/local/bin/pgcm proxy \
    --config-file /etc/pgsql-cluster-manager/config.toml \
    >>/var/log/pgsql-cluster-manager/proxy.log 2>&1 &
EOF

  chmod a+x /usr/local/bin/pgsql-cluster-manager.sh
  /usr/local/bin/pgsql-cluster-manager.sh
}

start_corosync
wait_for_quorum

if [ "$(hostname -i)" == "$PG01_IP" ]; then
  configure_pacemaker
fi

wait_for_roles
configure_dns # needs to happen before PgBouncer
start_etcd
start_pgbouncer
start_cluster_manager

log "Cluster is running"
