package checkpoint

//Provider provides container checkpoint to other services
type Provider interface {
	Prepare(ref string, target string) error
	Remove(target string) error
}
