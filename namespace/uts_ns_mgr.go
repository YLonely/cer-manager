package namespace

func NewUTSManager(capacity int) (Manager, error) {
	if mgr, err := newGenericManager(capacity, UTS, genericCreateNewNamespace); err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
