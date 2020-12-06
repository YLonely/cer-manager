package namespace

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/pkg/errors"
)

type NamespaceHelper struct {
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	Cmd      *exec.Cmd
	Ret      []byte
	cmdError bool
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
	if args != nil {
		for name, value := range args {
			cmd.Args = append(cmd.Args, "--"+name, value)
		}
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

func NewNamespaceExecEnterHelper(key NamespaceFunctionKey, nsType types.NamespaceType, fd int, args map[string]string) (*NamespaceHelper, error) {
	if fd <= 0 {
		return nil, errors.New("invalid fd value")
	}
	cmd := exec.Command("/proc/self/exe", "nsexec")
	if args != nil {
		for name, value := range args {
			cmd.Args = append(cmd.Args, "--"+name, value)
		}
	}
	cmd.Args = append(cmd.Args, string(key), string(nsType))
	cmd.Env = append(
		cmd.Env,
		nsexecOpKey+"="+nsexecOpEnter,
		nsexecNSTypeKey+"="+string(nsType),
		nsexecNSPathKey+"="+fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), fd),
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
			helper.Cmd.Process.Kill()
		}
	}()
	// read prefix from stdout, which also means the execution of cmd is finished
	prefix := make([]byte, 4)
	_, err = io.ReadFull(helper.stdout, prefix)
	if err != nil {
		return err
	}
	if string(prefix) == NamespaceErrorPrefix {
		helper.cmdError = true
	}
	if release {
		err = helper.Release()
		if err != nil {
			err = errors.Wrap(err, "failed to release child process")
			return
		}
	}
	return
}

// Release releases the child process
func (helper *NamespaceHelper) Release() error {
	io.WriteString(helper.stdin, "OK\n")
	content, err := ioutil.ReadAll(helper.stdout)
	if err := helper.Cmd.Wait(); err != nil {
		return err
	}
	if err != nil {
		return errors.Wrap(err, "failed to read stdout of cmd")
	}
	str := string(content)
	if helper.cmdError {
		return errors.New(str)
	}
	helper.Ret = content
	return nil
}
