package manager

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"

	log "github.com/sirupsen/logrus"
)

type NSType int

const (
	IPCNS NSType = iota
	MountNS
	UTSNS
)

func newGenericNSManager(capacity int, t NSType) (*genericNSManager, error) {
	if capacity <= 0 {
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
	t        NSType
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
	return nil
}

func (m *genericNSManager) init() error {
	var flag int
	var oldNSFd int
	var err error
	if flag, err = nsFlag(m.t); err != nil {
		return err
	}
	if oldNSFd, err = openNSFd(m.t); err != nil {
		return err
	}
	defer func() {
		if err = syscall.Close(oldNSFd); err != nil {
			log.Fatal(err)
		}
	}()
	for i := 0; i < m.capacity; i++ {
		if err = syscall.Unshare(flag); err != nil {
			m.CleanUp()
			return err
		}
	}

	return nil
}

func nsFlag(t NSType) (int, error) {
	switch t {
	case IPCNS:
		return syscall.CLONE_NEWIPC, nil
	case UTSNS:
		return syscall.CLONE_NEWUTS, nil
	case MountNS:
		return syscall.CLONE_NEWNS, nil
	default:
		return -1, errors.New("invalid ns type")
	}
}

func openNSFd(t NSType) (int, error) {
	var nsFileName string
	switch t {
	case IPCNS:
		nsFileName = "ipc"
	case UTSNS:
		nsFileName = "uts"
	case MountNS:
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
