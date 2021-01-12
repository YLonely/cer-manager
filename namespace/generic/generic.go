package generic

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/namespace"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func NewManager(capacity int, t types.NamespaceType, newNamespaceFunc func(types.NamespaceType) (f *os.File, err error)) (*GenericManager, error) {
	if capacity < 0 {
		return nil, errors.New("invalid capacity")
	}
	manager := &GenericManager{
		capacity:         capacity,
		usedNS:           map[int]*os.File{},
		t:                t,
		newNamespaceFunc: newNamespaceFunc,
	}
	if manager.newNamespaceFunc == nil {
		manager.newNamespaceFunc = genericCreateNewNamespace
	}
	if err := manager.init(); err != nil {
		return nil, err
	}
	return manager, nil
}

type GenericManager struct {
	capacity int
	// usedNS maps fd to file
	usedNS map[int]*os.File
	// unusedNS stores all unused namespace files
	unusedNS         []*os.File
	m                sync.Mutex
	t                types.NamespaceType
	newNamespaceFunc func(types.NamespaceType) (*os.File, error)
}

var _ namespace.Manager = &GenericManager{}

func (mgr *GenericManager) Get(interface{}) (fd int, info interface{}, err error) {
	mgr.m.Lock()
	defer mgr.m.Unlock()
	n := len(mgr.unusedNS)
	if n > 0 {
		file := mgr.unusedNS[n-1]
		mgr.unusedNS = mgr.unusedNS[:n-1]
		mgr.usedNS[int(file.Fd())] = file
		fd = int(file.Fd())
		return
	}
	err = errors.New("No namespace available")
	return
}

func (mgr *GenericManager) Put(fd int) error {
	mgr.m.Lock()
	defer mgr.m.Unlock()
	if f, exists := mgr.usedNS[fd]; !exists {
		return errors.Errorf("Namespace %d does not exists", fd)
	} else {
		delete(mgr.usedNS, fd)
		mgr.unusedNS = append(mgr.unusedNS, f)
	}
	return nil
}

func (mgr *GenericManager) Update(config interface{}) error {
	return nil
}

func (mgr *GenericManager) CleanUp() error {
	var failed []string
	files := make([]*os.File, 0, mgr.capacity)
	for _, f := range mgr.usedNS {
		files = append(files, f)
	}
	for _, f := range mgr.unusedNS {
		files = append(files, f)
	}
	for _, f := range files {
		if err := f.Close(); err != nil {
			failed = append(failed, fmt.Sprintf("%s %d", err.Error(), int(f.Fd())))
		}
	}
	if len(failed) != 0 {
		return errors.New(strings.Join(failed, ";"))
	}
	return nil
}

func (mgr *GenericManager) init() (err error) {
	var newNSFile *os.File
	defer func() {
		if err != nil {
			if errClean := mgr.CleanUp(); errClean != nil {
				err = errors.Wrap(err, errClean.Error())
			}
		}
	}()
	for i := 0; i < mgr.capacity; i++ {
		if newNSFile, err = mgr.newNamespaceFunc(mgr.t); err != nil {
			return
		} else {
			mgr.unusedNS = append(mgr.unusedNS, newNSFile)
		}
	}
	return
}

func genericCreateNewNamespace(t types.NamespaceType) (*os.File, error) {
	h, err := namespace.NewNamespaceExecCreateHelper("", t, nil)
	if err != nil {
		return nil, err
	}
	if err := h.Do(false); err != nil {
		return nil, errors.Wrapf(err, "failed to create namespace of type %s", string(t))
	}
	defer h.Release()
	nsFile, err := namespace.OpenNSFile(t, h.Cmd.Process.Pid)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open namespace file")
	}
	return nsFile, nil
}

func nsFlag(t types.NamespaceType) (int, error) {
	switch t {
	case types.NamespaceIPC:
		return unix.CLONE_NEWIPC, nil
	case types.NamespaceUTS:
		return unix.CLONE_NEWUTS, nil
	case types.NamespaceMNT:
		return unix.CLONE_NEWNS, nil
	default:
		return -1, errors.New("invalid ns type")
	}
}
