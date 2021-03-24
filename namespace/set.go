package namespace

import (
	"os"
)

func NewSet(capacity int, namespaceCreator func() (*os.File, error), preReleaseNamespace func(*os.File) error) (*Set, error) {
	files := map[int]*os.File{}
	for i := 0; i < capacity; i++ {
		f, err := namespaceCreator()
		if err != nil {
			return nil, err
		}
		files[int(f.Fd())] = f
	}
	return &Set{
		defaultCapacity:     capacity,
		files:               files,
		namespaceCreator:    namespaceCreator,
		preReleaseNamespace: preReleaseNamespace,
	}, nil
}

type Set struct {
	defaultCapacity     int
	files               map[int]*os.File
	namespaceCreator    func() (*os.File, error)
	preReleaseNamespace func(*os.File) error
}

func (s Set) Capacity() int {
	return len(s.files)
}

func (s Set) DefaultCapacity() int {
	return s.defaultCapacity
}

func (s *Set) Get() *os.File {
	for _, f := range s.files {
		ret := f
		delete(s.files, int(ret.Fd()))
		return ret
	}
	return nil
}

func (s *Set) CleanUp() error {
	var last error
	for fd, f := range s.files {
		if err := s.preReleaseNamespace(f); err != nil {
			last = err
		}
		f.Close()
		delete(s.files, fd)
	}
	return last
}

func (s *Set) CreateOne() error {
	f, err := s.namespaceCreator()
	if err != nil {
		return err
	}
	s.Add(f)
	return nil
}

func (s *Set) Add(f *os.File) {
	s.files[int(f.Fd())] = f
}

func (s *Set) Update(capacity int) error {
	cap := s.Capacity()
	diff := capacity - cap
	if diff == 0 {
		return nil
	}
	if diff > 0 {
		for i := 0; i < diff; i++ {
			if err := s.CreateOne(); err != nil {
				return err
			}
		}
	} else {
		diff = -diff
		if diff > cap {
			diff = cap
		}
		for i := 0; i < diff; i++ {
			f := s.Get()
			defer f.Close()
			if err := s.preReleaseNamespace(f); err != nil {
				return err
			}
		}
	}
	return nil
}
