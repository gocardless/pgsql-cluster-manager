#!/bin/bash

PG01="$1" # hostnames
PG02="$2"
PG03="$3"

cat <<EOF | crm configure
property stonith-enabled=true
property default-resource-stickiness=100
primitive PostgresqlVIP ocf:heartbeat:IPaddr2 params ip=172.17.0.99 cidr_netmask=32 op monitor interval=5s
primitive Postgresql ocf:heartbeat:pgsql \
    params pgctl="/usr/lib/postgresql/9.4/bin/pg_ctl" psql="/usr/bin/psql" \
    pgdata="/var/lib/postgresql/9.4/main/" start_opt="-p 5432" rep_mode="sync" \
    node_list="$PG01 $PG02 $PG03" primary_conninfo_opt="keepalives_idle=60 keepalives_interval=5 \
    keepalives_count=5" repuser="postgres" tmpdir="/var/lib/postgresql/9.4/tmp" \
    config="/etc/postgresql/9.4/main/postgresql.conf" \
    logfile="/var/log/postgresql/postgresql-crm.log" restore_command="exit 0" \
    master_ip="172.17.0.99" \
    op start timeout="60s" interval="0s" on-fail="restart" \
    op monitor timeout="60s" interval="2s" on-fail="restart" \
    op monitor timeout="60s" interval="1s" on-fail="restart" role="Master" \
    op promote timeout="60s" interval="0s" on-fail="restart" \
    op demote timeout="60s" interval="0s" on-fail="stop" \
    op stop timeout="60s" interval="0s" on-fail="block" \
    op notify timeout="60s" interval="0s"
ms msPostgresql Postgresql params master-max=1 master-node-max=1 clone-max=3 clone-node-max=1 notify=true
primitive shoot-pg01 stonith:external/docker params server_id="pg01"
location fence_pg01 shoot-pg01 -inf: pg01
primitive shoot-pg02 stonith:external/docker params server_id="pg02"
location fence_pg02 shoot-pg02 -inf: pg02
primitive shoot-pg03 stonith:external/docker params server_id="pg03"
location fence_pg03 shoot-pg03 -inf: pg03
colocation vip-with-master inf: PostgresqlVIP msPostgresql:Master
order start-vip-after-postgres inf: msPostgresql:promote PostgresqlVIP:start symmetrical=false
order stop-vip-after-postgres 0: msPostgresql:demote PostgresqlVIP:stop symmetrical=false
commit
end
EOF
