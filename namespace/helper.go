package namespace

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/pkg/errors"
)

type namespaceHelper struct {
	stdin io.WriteCloser
	cmd   *exec.Cmd
	ret   string
}

const (
	nsexecOpKey     string = "__OP_TYPE__"
	nsexecOpCreate  string = "CREATE"
	nsexecOpEnter   string = "ENTER"
	nsexecNSTypeKey string = "__NS_TYPE__"
	nsexecNSPathKey string = "__NS_PATH__"
)

func newNamespaceExecCreateHelper(key NamespaceFunctionKey, nsType types.NamespaceType, args map[string]string) (*namespaceHelper, error) {
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
	return &namespaceHelper{
		cmd: cmd,
	}, nil
}

func newNamespaceExecEnterHelper(key NamespaceFunctionKey, nsType types.NamespaceType, fd int, args map[string]string) (*namespaceHelper, error) {
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
	return &namespaceHelper{
		cmd: cmd,
	}, nil
}

func (helper *namespaceHelper) do() error {
	stdin, err := helper.cmd.StdinPipe()
	if err != nil {
		return errors.Wrap(err, "can't get stdin from cmd")
	}
	helper.stdin = stdin
	stdout, err := helper.cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "can't get stdout from cmd")
	}
	reader := bufio.NewReader(stdout)
	err = helper.cmd.Start()
	if err != nil {
		return errors.Wrap(err, "can't start cmd")
	}
	ret, err := reader.ReadString('\n')
	if err != nil {
		return errors.Wrap(err, "can't read ret value from cmd")
	}
	ret = strings.Trim(ret, "\n")
	if strings.HasPrefix(ret, NamespaceErrorPrefix) {
		return errors.Errorf("execute cmd", strings.TrimPrefix(ret, NamespaceErrorPrefix))
	}
	helper.ret = strings.TrimPrefix(ret, NamespaceReturnPrefix)
	return nil
}

func (helper *namespaceHelper) release() error {
	io.WriteString(helper.stdin, "OK\n")
	return helper.cmd.Wait()
}
