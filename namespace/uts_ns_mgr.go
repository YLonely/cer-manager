package namespace

func newUTSNamespaceManager(capacity int) (namespaceManager, error) {
	if mgr, err := newGenericNamespaceManager(capacity, UTS, nil); err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
