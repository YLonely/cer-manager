package rootfsserver

import (
	"errors"
	"sync"
)

func NewMountNSManager(capacity int, root string) (*mountNSManager, error) {
	if capacity <= 0 {
		return nil, errors.New("negative capacity")
	}
	return &mountNSManager{
		capacity:   capacity,
		root:       root,
		unused:     map[int]string{},
		used:       map[int]string{},
		initialled: false,
	}, nil
}

type mountNSManager struct {
	capacity   int
	root       string
	unused     map[int]string
	used       map[int]string
	m          sync.Mutex
	initialled bool
}
