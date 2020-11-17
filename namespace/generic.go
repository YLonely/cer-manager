package namespace

import (
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func newGenericManager(capacity int, t NamespaceType, newNamespaceFunc func(NamespaceType) (fd int, err error)) (*genericManager, error) {
	if capacity < 0 {
		return nil, errors.New("invalid capacity")
	}
	manager := &genericManager{
		capacity:         capacity,
		usedNS:           map[int]int{},
		unusedNS:         map[int]int{},
		t:                t,
		id:               0,
		newNamespaceFunc: newNamespaceFunc,
	}
	if err := manager.init(); err != nil {
		return nil, err
	}
	return manager, nil
}

type genericManager struct {
	capacity         int
	usedNS           map[int]int
	unusedNS         map[int]int
	id               int
	m                sync.Mutex
	t                NamespaceType
	newNamespaceFunc func(NamespaceType) (int, error)
}

var _ Manager = &genericManager{}

func (mgr *genericManager) Get(interface{}) (id int, newNSFd int, info interface{}, err error) {
	mgr.m.Lock()
	defer mgr.m.Unlock()
	if len(mgr.unusedNS) > 0 {
		for id, newNSFd = range mgr.unusedNS {
			delete(mgr.unusedNS, id)
			mgr.usedNS[id] = newNSFd
			return
		}
	}
	err = errors.New("No namespace available")
	return
}

func (mgr *genericManager) Put(id int) error {
	mgr.m.Lock()
	defer mgr.m.Unlock()
	if fd, exists := mgr.usedNS[id]; !exists {
		return errors.Errorf("Namespace %d does not exists", id)
	} else {
		delete(mgr.usedNS, id)
		mgr.unusedNS[id] = fd
	}
	return nil
}

func (mgr *genericManager) Update(config interface{}) error {
	return nil
}

func (mgr *genericManager) CleanUp() error {
	var err error
	fds := make([]int, 0, mgr.capacity)
	for _, fd := range mgr.usedNS {
		fds = append(fds, fd)
	}
	for _, fd := range mgr.unusedNS {
		fds = append(fds, fd)
	}
	for _, fd := range fds {
		if errClose := unix.Close(fd); errClose != nil {
			if err != nil {
				err = errors.Wrap(err, errClose.Error())
			} else {
				err = errClose
			}
		}
	}
	return err
}

func (mgr *genericManager) init() (err error) {
	var newNSFd int
	defer func() {
		if err != nil {
			if errClean := mgr.CleanUp(); errClean != nil {
				err = errors.Wrap(err, errClean.Error())
			}
		}
	}()
	for i := 0; i < mgr.capacity; i++ {
		if newNSFd, err = mgr.newNamespaceFunc(mgr.t); err != nil {
			return
		} else {
			mgr.unusedNS[mgr.id] = newNSFd
			mgr.id++
		}
	}
	return
}

func genericCreateNewNamespace(t NamespaceType) (int, error) {
	h, err := newNamespaceCreateHelper(t, "", "")
	if err != nil {
		return -1, err
	}
	if err := h.do(); err != nil {
		return -1, err
	}
	return h.getFd(), nil
}

func nsFlag(t NamespaceType) (int, error) {
	switch t {
	case IPC:
		return unix.CLONE_NEWIPC, nil
	case UTS:
		return unix.CLONE_NEWUTS, nil
	case MNT:
		return unix.CLONE_NEWNS, nil
	default:
		return -1, errors.New("invalid ns type")
	}
}
