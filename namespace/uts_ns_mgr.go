package namespace

import "github.com/YLonely/cer-manager/api/types"

func NewUTSManager(root string, capacity int) (Manager, error) {
	if mgr, err := newGenericManager(capacity, types.NamespaceUTS, genericCreateNewNamespace); err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
