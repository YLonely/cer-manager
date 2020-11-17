package namespace

func NewIPCManager(root string, capacity int) (Manager, error) {
	if mgr, err := newGenericManager(capacity, IPC, genericCreateNewNamespace); err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
