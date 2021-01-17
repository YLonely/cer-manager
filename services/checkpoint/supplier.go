package checkpoint

import "github.com/YLonely/cer-manager/api/types"

// Supplier provides the path of checkpoint files belongs to ref
type Supplier interface {
	Get(ref types.Reference) (string, error)
}
