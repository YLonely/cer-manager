package namespace

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func newGenericNamespaceManager(capacity int, t NamespaceType, postUnshareHook func() error) (*genericNamespaceManager, error) {
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
	hook     func() error
}

var _ namespaceManager = &genericNamespaceManager{}

func (mgr *genericNamespaceManager) Get(interface{}) (int, int, error) {
	mgr.m.Lock()
	defer mgr.m.Unlock()
	if len(mgr.unusedNS) > 0 {
		for id, fd := range mgr.unusedNS {
			delete(mgr.unusedNS, id)
			mgr.usedNS[id] = fd
			return id, fd, nil
		}
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	flag, _ := nsFlag(mgr.t)
	newNSFd, err := mgr.createNewNamespace(flag)
	if err != nil {
		return -1, -1, errors.Wrap(err, "failed to get namespace type:"+string(mgr.t))
	}
	mgr.usedNS[mgr.id] = newNSFd
	mgr.id++
	return mgr.id - 1, newNSFd, nil
}

func (mgr *genericNamespaceManager) Put(id int) error {
	mgr.m.Lock()
	defer mgr.m.Unlock()
	if fd, exists := mgr.usedNS[id]; !exists {
		return errors.Errorf("%d does not exists", id)
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
		if errClose := syscall.Close(fd); errClose != nil {
			err = errors.Wrap(errClose, "failed to clean up")
		}
	}
	return err
}

func (mgr *genericNamespaceManager) reduce() {
	mgr.m.Lock()
	defer mgr.m.Unlock()
	diff := len(mgr.unusedNS) + len(mgr.usedNS) - mgr.capacity
	ids := []int{}
	for id, _ := range mgr.unusedNS {
		ids = append(ids, id)
	}
	for i := 0; i < diff && i < len(ids); i++ {
		syscall.Close(mgr.unusedNS[ids[i]])
		delete(mgr.unusedNS, ids[i])
	}
}

func (mgr *genericNamespaceManager) init() (err error) {
	var flag int
	var oldNSFd, newNSFd int
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if flag, err = nsFlag(mgr.t); err != nil {
		return err
	}
	if oldNSFd, err = openNSFd(mgr.t); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if errClean := mgr.CleanUp(); errClean != nil {
				err = errors.Wrap(err, errClean.Error())
			}
		}
		if errClose := syscall.Close(oldNSFd); errClose != nil {
			err = errors.Wrap(err, errClose.Error())
		}
	}()
	for i := 0; i < mgr.capacity; i++ {
		if newNSFd, err = mgr.createNewNamespace(flag); err != nil {
			return err
		} else {
			mgr.unusedNS[mgr.id] = newNSFd
			mgr.id++
		}
	}
	//return back to the old ns
	err = unix.Setns(oldNSFd, flag)
	return err
}

func (mgr *genericNamespaceManager) createNewNamespace(flag int) (int, error) {
	if err := syscall.Unshare(flag); err != nil {
		return -1, err
	}
	if mgr.hook != nil {
		if err := mgr.hook(); err != nil {
			return -1, err
		}
	}
	return openNSFd(mgr.t)
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
