package uts

import (
	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/namespace"
	"github.com/YLonely/cer-manager/namespace/generic"
)

func NewManager(root string, capacities []int, refs []types.Reference) (namespace.Manager, error) {
	total := 0
	for _, c := range capacities {
		total += c
	}
	if mgr, err := generic.NewManager(total, types.NamespaceUTS, nil); err != nil {
		return nil, err
	} else {
		return mgr, nil
	}
}
