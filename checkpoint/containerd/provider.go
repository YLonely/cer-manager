package containerd

import (
	"context"
	"encoding/json"
	"os"
	"path"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/checkpoint"
	cd "github.com/containerd/containerd"
	"github.com/containerd/containerd/archive"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/platforms"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

//Config for the provider
type Config struct{}

// NewProvider returns a new checkpoint whose backend is containerd
func NewProvider(c Config) (checkpoint.Provider, error) {
	return &provider{}, nil
}

type provider struct {
}

var _ checkpoint.Provider = &provider{}

const (
	defaultContainerdAddress = "/run/containerd/containerd.sock"
	stateFile                = ".ready"
)

func (p *provider) Remove(target string) error {
	return nil
}

func (p *provider) Prepare(ref types.Reference, target string) error {
	stateFilePath := path.Join(target, stateFile)
	if _, err := os.Stat(stateFilePath); err == nil {
		return nil
	}
	ps := platforms.DefaultString()
	pt, err := platforms.Parse(ps)
	if err != nil {
		return err
	}
	client, err := cd.New(defaultContainerdAddress, cd.WithDefaultPlatform(platforms.Only(pt)))
	if err != nil {
		return errors.Wrap(err, "failed to create containerd client")
	}
	defer client.Close()
	ctx := namespaces.WithNamespace(context.Background(), ref.GetLabelWithKey("namespace"))
	image, err := client.GetImage(ctx, ref.Name)
	if err != nil {
		return errors.Wrap(err, "failed to get image from containerd")
	}
	store := client.ContentStore()
	index, err := decodeIndex(ctx, store, image.Target())
	if err != nil {
		return err
	}
	var descriptor *imagespec.Descriptor
	for _, m := range index.Manifests {
		if m.MediaType == images.MediaTypeContainerd1Checkpoint {
			descriptor = &m
			break
		}
	}
	if descriptor == nil {
		return errors.New("can't find checkpoint in image index")
	}
	reader, err := store.ReaderAt(ctx, *descriptor)
	if err != nil {
		return err
	}
	_, err = archive.Apply(ctx, target, content.NewReader(reader))
	reader.Close()
	if err != nil {
		return errors.Wrap(err, "failed to untar checkpoint files")
	}
	// create a .ready file in target dir which indecates the checkpoint files of ref is ready
	var f *os.File
	if f, err = os.Create(stateFilePath); err != nil {
		return errors.Wrap(err, "failed to create state file")
	}
	f.Close()
	os.Chmod(stateFilePath, 0755)
	return nil
}

func decodeIndex(ctx context.Context, store content.Provider, desc imagespec.Descriptor) (*imagespec.Index, error) {
	var index imagespec.Index
	p, err := content.ReadBlob(ctx, store, desc)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(p, &index); err != nil {
		return nil, err
	}
	return &index, nil
}
