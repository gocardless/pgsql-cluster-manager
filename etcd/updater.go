package etcd

import (
	"context"

	"github.com/coreos/etcd/clientv3"
)

type Updater struct {
	clientv3.KV
}

// Run will update the etcd key with the given value, but only if the value in etcd is
// different from our desired update. This avoids causing watchers that are subscribed to
// changes on this key triggering for multiple PUTs of the same value.
func (e Updater) Run(key, value string) error {
	txn := e.KV.Txn(context.Background()).
		If(
			clientv3.Compare(clientv3.Value(key), "=", value),
		).
		Else(
			clientv3.OpPut(key, value),
		)

	_, err := txn.Commit()
	return err
}
