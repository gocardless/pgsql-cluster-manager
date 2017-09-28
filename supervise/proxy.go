package supervise

import (
	"context"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/coreos/etcd/clientv3"
	"github.com/gocardless/pgsql-cluster-manager/etcd"
	"github.com/gocardless/pgsql-cluster-manager/pgbouncer"
)

func Proxy(
	ctx context.Context, // supervise only until the context expires
	logger *logrus.Logger, // log all output here
	client *clientv3.Client, // watch for changes using this etcd client
	bouncer pgbouncer.PGBouncer, // manage where this PGBouncer points to
	etcdHostKey string, // find the Postgres host at this key
) {
	etcd.NewSubscriber(client, etcd.WithLogger(logger)).
		AddHandler(etcdHostKey, &pgbouncer.HostChanger{bouncer, 5 * time.Second}).
		Start(ctx)
}
