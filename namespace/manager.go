package namespace

import "github.com/YLonely/cer-manager/api/types"

// Manager manages different types of namespace
type Manager interface {
	Get(ref types.Reference, extraRefs ...types.Reference) (fd int, info interface{}, err error)
	Put(fd int) error
	Update(ref types.Reference, capacity int) error
	CleanUp() error
}
