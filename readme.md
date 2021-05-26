# CER-Manager

> CER-Manager(**C**ontainer **E**xternal **R**esources Manager) is a service to manage external resources such as namespace and checkpoint files for containers.

# Build & Install

```
# make
# make install
```

# Prerequisite 

Update **containerd**, **runc** and **criu** with patches from branch [diffs](https://github.com/YLonely/cer-manager/tree/diffs).

Build and install the updated binaries according to the instructions given in the corresponding project.

[Build and install criu](https://criu.org/Installation)

[Build and install containerd](https://github.com/containerd/containerd/blob/master/BUILDING.md#build-containerd)

[Build and install runc](https://github.com/opencontainers/runc#building)

# Usage

## Use containerd to start a container

```
# ctr run -d --rm IMAGE_NAME test
```

## Use containerd to checkpoint a running container

```
# ctr c checkpoint --rw --image --task test CHECKPOINT_NAME
```

## Create the config file for cer-manager

```
## create the root directory if not exist
# mkdir -p /var/lib/cermanager 
# cat <<EOF > /var/lib/cermanager/namespace_service.json
{
    "containerd_checkpoints":[
        {
            "name":"CHECKPOINT_NAME",
            "namespace":"default"
        }
    ],
    "default_capacity":10
}
```

The `namespace_service.json` contains the name of the container checkpoint that needs to be managed by cer-manager and the namespace to which the checkpoint it belongs. 
The field `default_capacity` indicates the number of isolation resources initially available for each checkpoint.

## Start the cer-manager
```
# cermanager [--debug] start
```

## Restore a container
Restore a container from the checkpoint with the isolation resources provided by cer-manager.

```
# ctr c restore --live --external-ns ipc --external-ns uts --external-ns mnt [--external-checkpoint] CHECKPOINT_NAME test-restore
```

The flag `--external-checkpoint` prompts containerd to use the checkpoint resources provided by cer-manager instead of temporarily decompressing the checkpoint when restoring the container