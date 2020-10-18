package namespace

import "time"

func newUTSNamespaceManager(capacity int) (namespaceManager, error) {
	if mgr, err := newGenericNamespaceManager(capacity, UTS, nil); err != nil {
		return nil, err
	} else {
		go func() {
			for {
				mgr.reduce()
				time.Sleep(10 * time.Second)
			}
		}()
		return mgr, nil
	}
}
