package namespace

import (
	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/services"
)

func NewIPCManager(root string, capacity int, imageRefs []string, supplier services.CheckpointSupplier) (Manager, error) {
	if mgr, err := newGenericManager(capacity, types.NamespaceIPC, genericCreateNewNamespace); err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
