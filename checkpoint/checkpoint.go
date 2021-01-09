package checkpoint

// Provider provides container checkpoints to other components
type Provider interface {
	Prepare(checkpointName string, target string) error
	Remove(target string) error
}

// ReferenceManager manages the references on checkpoint
type ReferenceManager interface {
	Add(checkpointName string)
	Release(checkpointName string)
}
