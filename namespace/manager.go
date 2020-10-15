package namespace

type NSManager interface {
	Get() (int, int, interface{}, error)
	Put(int) error
	Update(interface{}) error
	CleanUp() error
}
