package namespace

// Manager manages different types of namespace
type Manager interface {
	Get(arg interface{}) (fd int, info interface{}, err error)
	Put(fd int) error
	Update(interface{}) error
	CleanUp() error
}
