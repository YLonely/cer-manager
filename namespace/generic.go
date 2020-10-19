package namespace

import (
	"fmt"
	"os"
	"runtime"
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func newGenericNamespaceManager(capacity int, t NamespaceType, postUnshareHook func(oldNS, newNS int) error) (*genericNamespaceManager, error) {
	if capacity < 0 {
		return nil, errors.New("invalid capacity")
	}
	manager := &genericNamespaceManager{
		capacity: capacity,
		usedNS:   map[int]int{},
		unusedNS: map[int]int{},
		t:        t,
		id:       0,
		hook:     postUnshareHook,
	}
	if err := manager.init(); err != nil {
		return nil, err
	}
	return manager, nil
}

type genericNamespaceManager struct {
	capacity int
	usedNS   map[int]int
	unusedNS map[int]int
	id       int
	m        sync.Mutex
	t        NamespaceType
	hook     func(int, int) error
}

var _ namespaceManager = &genericNamespaceManager{}

func (mgr *genericNamespaceManager) Get(interface{}) (id int, newNSFd int, info interface{}, err error) {
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

func (mgr *genericNamespaceManager) Put(id int) error {
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

func (mgr *genericNamespaceManager) Update(config interface{}) error {
	return nil
}

func (mgr *genericNamespaceManager) CleanUp() error {
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

func (mgr *genericNamespaceManager) init() (err error) {
	var flag int
	var oldNSFd, newNSFd int
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if flag, err = nsFlag(mgr.t); err != nil {
		return
	}
	if oldNSFd, err = openNSFd(mgr.t); err != nil {
		return
	}
	defer func() {
		if err != nil {
			if errClean := mgr.CleanUp(); errClean != nil {
				err = errors.Wrap(err, errClean.Error())
			}
		}
		if errClose := unix.Close(oldNSFd); errClose != nil {
			if err != nil {
				err = errors.Wrap(err, errClose.Error())
			} else {
				err = errClose
			}
		}
	}()
	for i := 0; i < mgr.capacity; i++ {
		if newNSFd, err = mgr.createNewNamespace(flag, oldNSFd); err != nil {
			return
		} else {
			mgr.unusedNS[mgr.id] = newNSFd
			mgr.id++
		}
	}
	//return back to the old ns
	err = unix.Setns(oldNSFd, flag)
	return
}

func (mgr *genericNamespaceManager) createNewNamespace(flag, oldNS int) (int, error) {
	var err error
	if err = unix.Unshare(flag); err != nil {
		return -1, err
	}
	fd, errOpen := openNSFd(mgr.t)
	if errOpen != nil {
		if err != nil {
			err = errors.Wrap(err, errOpen.Error())
		} else {
			err = errOpen
		}
	}
	if mgr.hook != nil {
		if err = mgr.hook(oldNS, fd); err != nil {
			err = errors.Wrap(err, "failed to execute the hook")
		}
	}
	return fd, err
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
