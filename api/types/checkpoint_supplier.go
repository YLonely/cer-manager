package types

// Supplier provides the path of checkpoint files belongs to ref
type Supplier interface {
	Get(ref Reference) (string, error)
}
