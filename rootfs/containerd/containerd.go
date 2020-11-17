package containerd

import (
	"context"
	"encoding/json"

	"github.com/YLonely/cer-manager/mount"
	"github.com/YLonely/cer-manager/rootfs"
	cd "github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	mnt "github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/platforms"
	"github.com/opencontainers/image-spec/identity"
	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// NewProvider returns a rootfs provider which use containerd as backend
func NewProvider() (rootfs.Provider, error) {
	return &provider{}, nil
}

type provider struct {
}

var _ rootfs.Provider = &provider{}

const (
	defaultContainerdAddress       = "/run/containerd/containerd.sock"
	checkpointImageNameLabel       = "org.opencontainers.image.ref.name"
	checkpointSnapshotterNameLabel = "io.containerd.checkpoint.snapshotter"
)

func (p *provider) Prepare(name, key string) ([]mount.Mount, error) {
	ps := platforms.DefaultString()
	pt, err := platforms.Parse(ps)
	if err != nil {
		return nil, err
	}
	client, err := cd.New(defaultContainerdAddress, cd.WithDefaultPlatform(platforms.Only(pt)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create containerd client")
	}
	defer client.Close()
	ctx := namespaces.WithNamespace(context.Background(), "default")
	checkpoint, err := client.GetImage(ctx, name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get image")
	}
	store := client.ContentStore()
	index, err := decodeIndex(ctx, store, checkpoint.Target())
	if err != nil {
		return nil, err
	}
	baseImageName, exists := index.Annotations[checkpointImageNameLabel]
	if !exists || baseImageName == "" {
		return nil, errors.Errorf("%s is not a container checkpoint", name)
	}
	snapshotter, exists := index.Annotations[checkpointSnapshotterNameLabel]
	if !exists || snapshotter == "" {
		return nil, errors.Errorf("Can't find snapshotter in image %s", name)
	}
	baseImage, err := client.GetImage(ctx, baseImageName)
	if err != nil {
		return nil, err
	}
	if err = baseImage.Unpack(ctx, snapshotter); err != nil {
		return nil, errors.Wrap(err, "error unpacking image")
	}
	diffIDs, err := baseImage.RootFS(ctx)
	if err != nil {
		return nil, err
	}
	chainID := identity.ChainID(diffIDs).String()
	snapshotClient := client.SnapshotService(snapshotter)
	var mounts []mnt.Mount
	mounts, err = snapshotClient.Prepare(ctx, key, chainID)
	if err != nil {
		if errdefs.IsAlreadyExists(err) {
			mounts, err = snapshotClient.Mounts(ctx, key)
			if err != nil {
				return nil, err
			}
		}
	}
	rw, err := cd.GetIndexByMediaType(index, imagespec.MediaTypeImageLayerGzip)
	if err != nil {
		return nil, err
	}
	if _, err = client.DiffService().Apply(ctx, *rw, mounts); err != nil {
		return nil, err
	}
	ret := []mount.Mount{}
	for _, m := range mounts {
		ret = append(ret, mount.Mount(m))
	}
	return ret, nil
}

func (p *provider) Remove(key string) error {
	client, err := cd.New(defaultContainerdAddress)
	if err != nil {
		return err
	}
	defer client.Close()
	ctx := namespaces.WithNamespace(context.Background(), "default")
	sClient := client.SnapshotService(cd.DefaultSnapshotter)
	if err = sClient.Remove(ctx, key); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
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
