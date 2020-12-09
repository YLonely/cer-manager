package uts

import (
	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/namespace"
	"github.com/YLonely/cer-manager/namespace/generic"
)

func NewManager(root string, capacity int, imageRefs []string) (namespace.Manager, error) {
	if mgr, err := generic.NewManager(capacity*len(imageRefs), types.NamespaceUTS, nil); err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
