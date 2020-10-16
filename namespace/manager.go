package namespace

type namespaceManager interface {
	Get(arg interface{}) (int, int, error)
	Put(int) error
	Update(interface{}) error
	CleanUp() error
}
