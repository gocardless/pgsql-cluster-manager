#!/usr/bin/env bash

function run-container() {
  local id
  id=$(
  docker run \
    --name "$1" \
    --hostname "$1" \
    --privileged \
    --detach \
    -v "$(git rev-parse --show-toplevel)/pgsql-cluster-manager_0.0.1_amd64.deb:/pgsql-cluster-manager.deb" \
    postgres-member \
    sh -c "while :; do sleep 1; done")
  docker inspect "$id" | jq -r '.[0].NetworkSettings.IPAddress'
}

docker rm pg01 pg02 pg03

PG01=$(run-container pg01)
PG02=$(run-container pg02)
PG03=$(run-container pg03)

docker exec --detach pg01 /bin/start-cluster "$PG01" "$PG02" "$PG03"
docker exec --detach pg02 /bin/start-cluster "$PG01" "$PG02" "$PG03"
docker exec --detach pg03 /bin/start-cluster "$PG01" "$PG02" "$PG03"

docker exec -it pg01 /bin/bash

docker kill pg01 pg02 pg03
docker rm pg01 pg02 pg03