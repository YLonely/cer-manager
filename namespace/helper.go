package namespace

import (
	"io"
	"os/exec"
	"strconv"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/pkg/errors"
)

type NamespaceHelper struct {
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	Cmd      *exec.Cmd
	Ret      []byte
	released bool
}

const (
	nsexecOpKey     string = "__OP_TYPE__"
	nsexecOpCreate  string = "CREATE"
	nsexecOpEnter   string = "ENTER"
	nsexecNSTypeKey string = "__NS_TYPE__"
	nsexecNSPathKey string = "__NS_PATH__"
)

func NewNamespaceExecCreateHelper(key NamespaceFunctionKey, nsType types.NamespaceType, args map[string]string) (*NamespaceHelper, error) {
	cmd := exec.Command("/proc/self/exe", "nsexec")
	for name, value := range args {
		cmd.Args = append(cmd.Args, "--"+name, value)
	}
	cmd.Args = append(cmd.Args, string(key), string(nsType))
	cmd.Env = append(
		cmd.Env,
		nsexecOpKey+"="+nsexecOpCreate,
		nsexecNSTypeKey+"="+string(nsType),
	)
	return &NamespaceHelper{
		Cmd: cmd,
	}, nil
}

func NewNamespaceExecEnterHelper(key NamespaceFunctionKey, nsType types.NamespaceType, fdPath string, args map[string]string) (*NamespaceHelper, error) {
	if fdPath == "" {
		return nil, errors.New("empty namespace fd path")
	}
	cmd := exec.Command("/proc/self/exe", "nsexec")
	for name, value := range args {
		cmd.Args = append(cmd.Args, "--"+name, value)
	}
	cmd.Args = append(cmd.Args, string(key), string(nsType))
	cmd.Env = append(
		cmd.Env,
		nsexecOpKey+"="+nsexecOpEnter,
		nsexecNSTypeKey+"="+string(nsType),
		nsexecNSPathKey+"="+fdPath,
	)
	return &NamespaceHelper{
		Cmd: cmd,
	}, nil
}

// Do executes the command
func (helper *NamespaceHelper) Do(release bool) (err error) {
	helper.stdin, err = helper.Cmd.StdinPipe()
	if err != nil {
		err = errors.Wrap(err, "can't get stdin from cmd")
		return
	}
	helper.stdout, err = helper.Cmd.StdoutPipe()
	if err != nil {
		err = errors.Wrap(err, "can't get stdout from cmd")
	}
	err = helper.Cmd.Start()
	if err != nil {
		err = errors.Wrap(err, "can't start cmd")
		return
	}
	defer func() {
		if err != nil {
			helper.forceRelease()
		}
	}()
	// read output of child process
	var (
		prefix  []byte
		content []byte
	)
	prefix, content, err = readContent(helper.stdout)
	if err != nil {
		err = errors.Wrap(err, "failed to read output of child process")
		return
	}
	if string(prefix) == NamespaceErrorPrefix {
		err = errors.Wrap(errors.New(string(content)), "child process returns an error")
		return
	}
	helper.Ret = content
	if release {
		err = helper.Release()
		if err != nil {
			err = errors.Wrap(err, "release child process with error")
			return
		}
	}
	return
}

// Release releases the child process and get the return content from child process
func (helper *NamespaceHelper) Release() error {
	if _, err := io.WriteString(helper.stdin, "OK\n"); err != nil {
		helper.forceRelease()
		return errors.Wrap(err, "failed to send exit signal to child process")
	}
	helper.released = true
	if err := helper.Cmd.Wait(); err != nil {
		return errors.Wrap(err, "failed to wait child process")
	}
	return nil
}

func (helper *NamespaceHelper) forceRelease() {
	if !helper.released {
		helper.Cmd.Process.Kill()
		helper.Cmd.Wait()
		helper.released = true
	}
}

func readContent(r io.Reader) ([]byte, []byte, error) {
	prefix := make([]byte, 4)
	_, err := io.ReadFull(r, prefix)
	if err != nil {
		return nil, nil, errors.Wrap(err, "can't read any byte from cmd's stdout")
	}
	sizeBytes := []byte{}
	b := make([]byte, 1)
	for {
		if _, err := r.Read(b); err != nil {
			return nil, nil, err
		}
		if b[0] == ',' {
			break
		}
		sizeBytes = append(sizeBytes, b[0])
	}
	size, err := strconv.Atoi(string(sizeBytes))
	if err != nil {
		return nil, nil, err
	}
	if size == 0 {
		return prefix, []byte{}, nil
	}
	content := make([]byte, size)
	_, err = io.ReadFull(r, content)
	if err != nil {
		return nil, nil, err
	}
	return prefix, content, nil
}
