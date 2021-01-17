package types

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
)

func NewContainerdReference(name string, namespace string) Reference {
	if namespace == "" {
		namespace = "default"
	}
	return Reference{
		Name: name,
		Labels: map[string]string{
			"namespace": namespace,
		},
	}
}

// Reference represents a checkpoint or an image in cer-manager
type Reference struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

func (ref Reference) GetLabelWithKey(key string) string {
	return ref.Labels[key]
}

func (ref Reference) Digest() string {
	bs, err := json.Marshal(ref)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(bs))
}

func (ref Reference) String() string {
	labels := []string{}
	for k, v := range ref.Labels {
		labels = append(labels, k+":"+v)
	}
	return fmt.Sprintf("(%s)%s", strings.Join(labels, ","), ref.Name)
}
