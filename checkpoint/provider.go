package checkpoint

//Provider provides container checkpoints to other services
type Provider interface {
	Prepare(ref string, target string) error
	Remove(target string) error
}
