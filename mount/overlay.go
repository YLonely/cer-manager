package mount

import (
	"strings"

	"github.com/pkg/errors"
)

var (
	OverlayTypeMismatchError = errors.New("not overlay type")
)

const (
	upperPrefix = "upperdir="
	workPrefix  = "workdir="
	lowerPrefix = "lowerdir="
)

func (m *Mount) IsOverlay() bool {
	return m.Type == "overlay"
}

func (m *Mount) Upper() string {
	return findValueWithPrefix(upperPrefix, m.Options)
}

func (m *Mount) Work() string {
	return findValueWithPrefix(workPrefix, m.Options)
}

func (m *Mount) Lowers() []string {
	lower := findValueWithPrefix(lowerPrefix, m.Options)
	return strings.Split(lower, ":")
}

func (m *Mount) SetUpper(upper string) {
	m.setOptions(upperPrefix, upper)
}

func (m *Mount) SetWork(work string) {
	m.setOptions(workPrefix, work)
}

func (m *Mount) SetLowers(lowers []string) {
	if len(lowers) == 0 {
		return
	}
	m.setOptions(lowerPrefix, strings.Join(lowers, ":"))
}

func (m *Mount) setOptions(prefix, value string) {
	exists := false
	for i, str := range m.Options {
		if strings.HasPrefix(str, prefix) {
			exists = true
			if value == "" {
				before := m.Options[:i]
				after := m.Options[i+1:]
				m.Options = append(append([]string{}, before...), after...)
			} else {
				m.Options[i] = prefix + value
			}
			break
		}
	}
	if !exists {
		m.Options = append(m.Options, prefix+value)
	}
}

func findValueWithPrefix(prefix string, options []string) string {
	for _, o := range options {
		if strings.HasPrefix(o, prefix) {
			return strings.TrimPrefix(o, prefix)
		}
	}
	return ""
}
