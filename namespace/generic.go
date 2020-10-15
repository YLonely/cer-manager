package namespace

import (
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/pkg/errors"
)

func newGenericNSManager(capacity int, t NamespaceType) (*genericNSManager, error) {
	if capacity < 0 {
		return nil, errors.New("invalid capacity")
	}
	manager := &genericNSManager{
		capacity: capacity,
		usedNS:   make([]int, 0, capacity),
		unusedNS: make([]int, 0, capacity),
		t:        t,
	}
	if err := manager.init(); err != nil {
		return nil, err
	}
	return manager, nil
}

type genericNSManager struct {
	capacity int
	usedNS   []int
	unusedNS []int
	m        sync.Mutex
	t        NamespaceType
}

var _ NSManager = &genericNSManager{}

func (m *genericNSManager) Get() (int, int, interface{}, error) {
	return 0, 0, nil, nil
}

func (m *genericNSManager) Put(id int) error {
	return nil
}

func (m *genericNSManager) Update(config interface{}) error {
	return nil
}

func (m *genericNSManager) CleanUp() error {
	var err error

}

func (m *genericNSManager) init() (err error) {
	var flag int
	var oldNSFd, newNSFd int
	if flag, err = nsFlag(m.t); err != nil {
		return err
	}
	if oldNSFd, err = openNSFd(m.t); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if errClean := m.CleanUp(); errClean != nil {
				err = errors.Wrap(err, errClean.Error())
			}
		}
		if errClose := syscall.Close(oldNSFd); errClose != nil {
			err = errors.Wrap(err, errClose.Error())
		}
	}()
	for i := 0; i < m.capacity; i++ {
		if err = syscall.Unshare(flag); err != nil {
			return err
		}
		if newNSFd, err = openNSFd(m.t); err != nil {
			return err
		} else {
			m.unusedNS = append(m.unusedNS, newNSFd)
		}
	}
	return err
}

func nsFlag(t NamespaceType) (int, error) {
	switch t {
	case IPC:
		return syscall.CLONE_NEWIPC, nil
	case UTS:
		return syscall.CLONE_NEWUTS, nil
	case MNT:
		return syscall.CLONE_NEWNS, nil
	default:
		return -1, errors.New("invalid ns type")
	}
}

func openNSFd(t NamespaceType) (int, error) {
	var nsFileName string
	switch t {
	case IPC:
		nsFileName = "ipc"
	case UTS:
		nsFileName = "uts"
	case MNT:
		nsFileName = "mnt"
	default:
		return -1, errors.New("invalid ns type")
	}
	pid := os.Getpid()
	nsFilePath := fmt.Sprintf("/proc/%d/ns/%s", pid, nsFileName)
	f, err := os.Open(nsFilePath)
	if err != nil {
		return -1, err
	}
	return int(f.Fd()), nil
}
