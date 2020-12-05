package namespace

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func newGenericManager(capacity int, t types.NamespaceType, newNamespaceFunc func(types.NamespaceType) (f *os.File, err error)) (*genericManager, error) {
	if capacity < 0 {
		return nil, errors.New("invalid capacity")
	}
	manager := &genericManager{
		capacity:         capacity,
		usedNS:           map[int]*os.File{},
		unusedNS:         map[int]*os.File{},
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
	capacity int
	// usedNS maps id to already used namespaces
	usedNS map[int]*os.File
	// unusedNS maps id to unused namespaces
	unusedNS         map[int]*os.File
	id               int
	m                sync.Mutex
	t                types.NamespaceType
	newNamespaceFunc func(types.NamespaceType) (*os.File, error)
}

var _ Manager = &genericManager{}

func (mgr *genericManager) Get(interface{}) (id int, newNSFd int, info interface{}, err error) {
	var f *os.File
	mgr.m.Lock()
	defer mgr.m.Unlock()
	if len(mgr.unusedNS) > 0 {
		for id, f = range mgr.unusedNS {
			delete(mgr.unusedNS, id)
			mgr.usedNS[id] = f
			newNSFd = int(f.Fd())
			return
		}
	}
	err = errors.New("No namespace available")
	return
}

func (mgr *genericManager) Put(id int) error {
	mgr.m.Lock()
	defer mgr.m.Unlock()
	if f, exists := mgr.usedNS[id]; !exists {
		return errors.Errorf("Namespace %d does not exists", id)
	} else {
		delete(mgr.usedNS, id)
		mgr.unusedNS[id] = f
	}
	return nil
}

func (mgr *genericManager) Update(config interface{}) error {
	return nil
}

func (mgr *genericManager) CleanUp() error {
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

func (mgr *genericManager) init() (err error) {
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
			mgr.unusedNS[mgr.id] = newNSFile
			mgr.id++
		}
	}
	return
}

func genericCreateNewNamespace(t types.NamespaceType) (*os.File, error) {
	h, err := newNamespaceExecCreateHelper("", t, nil)
	if err != nil {
		return nil, err
	}
	if err := h.do(); err != nil {
		return nil, errors.Wrapf(err, "failed to create namespace of type %s", string(t))
	}
	nsFile, err := OpenNSFile(t, h.cmd.Process.Pid)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open namespace file")
	}
	err = h.release()
	if err != nil {
		return nil, errors.Wrap(err, "failed to release child process")
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
