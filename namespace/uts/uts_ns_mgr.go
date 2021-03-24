package uts

import (
	"os"
	"sync"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/namespace"
	"github.com/pkg/errors"
)

func NewManager(capacities []int, refs []types.Reference) (namespace.Manager, error) {
	m := &manager{
		sets: map[string]*namespace.Set{},
		usedNamespace: map[int]struct {
			ref types.Reference
			f   *os.File
		}{},
	}
	for i, ref := range refs {
		if err := m.initSet(ref, capacities[i]); err != nil {
			return nil, err
		}
	}
	return m, nil
}

type manager struct {
	m             sync.Mutex
	sets          map[string]*namespace.Set
	usedNamespace map[int]struct {
		ref types.Reference
		f   *os.File
	}
}

func (m *manager) Get(ref types.Reference, extraRefs ...types.Reference) (fd int, info interface{}, err error) {
	if len(extraRefs) != 0 {
		err = errors.New("extra references is not supported")
		return
	}
	m.m.Lock()
	defer m.m.Unlock()
	set, exists := m.sets[ref.Digest()]
	if !exists {
		err = errors.Errorf("UTS namespaces of ref %s does not exist", ref)
		return
	}
	f := set.Get()
	if f == nil {
		err = errors.Errorf("UTS namespace of ref %s is used up")
		return
	}
	// Keep the number of namespace resources at the set value(capacity)
	go func() {
		m.m.Lock()
		defer m.m.Unlock()
		if set.Capacity() >= set.DefaultCapacity() {
			return
		}
		if err := set.CreateOne(); err != nil {
			log.Raw().WithError(err).Errorf("failed to create a new UTS namespace for %s", ref)
		}
	}()
	fd = int(f.Fd())
	m.usedNamespace[fd] = struct {
		ref types.Reference
		f   *os.File
	}{
		ref: ref,
		f:   f,
	}
	return
}

func (m *manager) Put(fd int) error {
	m.m.Lock()
	defer m.m.Unlock()
	item, exists := m.usedNamespace[fd]
	if !exists {
		return errors.Errorf("invalid namespace fd %d", fd)
	}
	set, exists := m.sets[item.ref.Digest()]
	if !exists {
		panic(errors.Errorf("namespace set of ref %s does not exist", item.ref))
	}
	set.Add(item.f)
	delete(m.usedNamespace, fd)
	return nil
}

func (m *manager) initSet(ref types.Reference, capacity int) error {
	set, err := namespace.NewSet(capacity, newUTSNamespace, func(f *os.File) error { return nil })
	if err != nil {
		return errors.Errorf("failed to create namespace set for ref %s", ref)
	}
	m.sets[ref.Digest()] = set
	return nil
}

func (m *manager) Update(ref types.Reference, capacity int) error {
	m.m.Lock()
	defer m.m.Unlock()
	set, exists := m.sets[ref.Digest()]
	if !exists {
		if err := m.initSet(ref, capacity); err != nil {
			return err
		}
		return nil
	}
	return set.Update(capacity)
}

func (m *manager) CleanUp() error {
	var last error
	for digest, set := range m.sets {
		if err := set.CleanUp(); err != nil {
			last = err
			log.Raw().WithError(err).Errorf("failed to clean up the UTS namespace set of %s", digest)
		}
	}
	for _, item := range m.usedNamespace {
		log.Raw().Warnf("namespace file %d is being used", item.f.Fd())
		item.f.Close()
	}
	return last
}

func newUTSNamespace() (*os.File, error) {
	h, err := namespace.NewNamespaceExecCreateHelper("", types.NamespaceUTS, nil)
	if err != nil {
		return nil, err
	}
	if err := h.Do(false); err != nil {
		return nil, errors.Wrap(err, "failed to create new UTS namespace")
	}
	defer h.Release()
	nsFile, err := namespace.OpenNSFile(types.NamespaceUTS, h.Cmd.Process.Pid)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open UTS namespace")
	}
	return nsFile, nil
}
