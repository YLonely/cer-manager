package namespace

func NewIPCManager(capacity int) (Manager, error) {
	if mgr, err := newGenericManager(capacity, IPC, genericCreateNewNamespace); err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
