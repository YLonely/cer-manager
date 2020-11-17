package namespace

// Manager manages different types of namespace
type Manager interface {
	Get(arg interface{}) (id int, fd int, info interface{}, err error)
	Put(int) error
	Update(interface{}) error
	CleanUp() error
}
