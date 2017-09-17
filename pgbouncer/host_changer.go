package pgbouncer

type HostChanger struct {
	PGBouncer
}

// Run receives new PGBouncer host values and will reload PGBouncer to point at the new
// host
func (h HostChanger) Run(_, host string) error {
	err := h.GenerateConfig(host)

	if err != nil {
		return err
	}

	return h.Reload()
}
