package checkpoint

import "github.com/YLonely/cer-manager/api/types"

// Provider provides container checkpoints to other components
type Provider interface {
	Prepare(ref types.Reference, target string) error
	Remove(target string) error
}

// SharedManager manages the shared use on a Reference
type SharedManager interface {
	Add(ref types.Reference)
	Release(ref types.Reference)
}
