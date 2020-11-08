package namespace

func newIPCNamespaceManager(capacity int) (namespaceManager, error) {
	if mgr, err := newGenericNamespaceManager(capacity, IPC, genericCreateNewNamespace); err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
