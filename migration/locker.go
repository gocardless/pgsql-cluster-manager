package migration

import "context"

func (m *migration) Lock() error {
	ctx, cancel := context.WithTimeout(context.Background(), m.lockTimeout)
	defer cancel()

	m.logger.Info("Acquiring etcd lock")
	return m.locker.Lock(ctx)
}

func (m *migration) Unlock() error {
	ctx, cancel := context.WithTimeout(context.Background(), m.lockTimeout)
	defer cancel()

	m.logger.Info("Releasing etcd lock")
	return m.locker.Unlock(ctx)
}
