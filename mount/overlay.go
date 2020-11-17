package mount

import (
	"strings"

	"github.com/pkg/errors"
)

// Overlay represents a overlay mount
type Overlay struct {
	Mount
	upperDir  string
	workDir   string
	lowerDirs []string
}

var (
	OverlayTypeMismatchError = errors.New("not overlay type")
)

const (
	upperPrefix = "upperdir="
	workPrefix  = "workdir="
	lowerPrefix = "lowerdir="
)

func NewOverlay(lowers []string, upper string, work string, options ...string) Overlay {
	ret := Overlay{
		lowerDirs: lowers,
		upperDir:  upper,
		workDir:   work,
	}
	opts := []string{
		lowerPrefix + strings.Join(lowers, ":"),
		upperPrefix + upper,
		workPrefix + work,
	}
	opts = append(opts, options...)
	ret.Source = "overlay"
	ret.Type = "overlay"
	ret.Options = opts
	return ret
}

func (o *Overlay) Upper() string {
	return o.upperDir
}

func (o *Overlay) Work() string {
	return o.workDir
}

func (o *Overlay) Lowers() []string {
	return o.lowerDirs
}

func (o *Overlay) SetUpper(upper string) {
	o.upperDir = upper
	o.setOptions(upperPrefix, upper)
}

func (o *Overlay) SetWork(work string) {
	o.workDir = work
	o.setOptions(workPrefix, work)
}

func (o *Overlay) SetLowers(lowers []string) {
	o.lowerDirs = lowers
	o.setOptions(lowerPrefix, strings.Join(lowers, ":"))
}

func (o *Overlay) setOptions(prefix, value string) {
	for i, str := range o.Options {
		if strings.HasPrefix(str, prefix) {
			o.Options[i] = prefix + value
		}
	}
}

func ToOverlay(m Mount) (Overlay, error) {
	if m.Type != "overlay" {
		return Overlay{}, OverlayTypeMismatchError
	}
	var err error
	ret := Overlay{
		Mount: m,
	}
	ret.upperDir, ret.workDir, ret.lowerDirs, err = parseOverlayOptions(m.Options)
	if err != nil {
		return Overlay{}, err
	}
	return ret, nil
}

func ToOverlays(ms []Mount) ([]Overlay, error) {
	os := []Overlay{}
	for _, m := range ms {
		o, err := ToOverlay(m)
		if err != nil {
			return []Overlay{}, err
		}
		os = append(os, o)
	}
	return os, nil
}

func parseOverlayOptions(options []string) (upper string, work string, lowers []string, err error) {
	for _, o := range options {
		if strings.HasPrefix(o, upperPrefix) {
			upper = strings.TrimPrefix(o, upperPrefix)
		} else if strings.HasPrefix(o, lowerPrefix) {
			lowerOption := strings.TrimPrefix(o, lowerPrefix)
			lowers = strings.Split(lowerOption, ":")
		} else if strings.HasPrefix(o, workPrefix) {
			work = strings.TrimPrefix(o, workPrefix)
		}
	}
	if len(lowers) == 0 {
		err = errors.New("Invalid overlay mount options")
		return
	}
	return
}
