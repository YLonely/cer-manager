package namespace

func NewIPCNamespaceManager(capacity int) (namespaceManager, error) {
	if mgr, err := newGenericNamespaceManager(capacity, IPC, nil); err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
