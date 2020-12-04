package namespace

import "github.com/YLonely/cer-manager/api/types"

func NewIPCManager(root string, capacity int, imageRefs []string) (Manager, error) {
	if mgr, err := newGenericManager(capacity, types.NamespaceIPC, genericCreateNewNamespace); err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
