package monad

// CollectError wraps various potentially failable functions, exiting early and returning
// the error if any fail
func CollectError(statements ...func() error) error {
	for _, statement := range statements {
		if err := statement(); err != nil {
			return err
		}
	}

	return nil
}
