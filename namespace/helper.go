package namespace

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type namespaceHelper struct {
	newNSFd int
	cmd     *exec.Cmd
	t       NamespaceType
	op      NamespaceOpType
}

const (
	nsexecOpKey     string = "OP_TYPE"
	nsexecOpCreate  string = "CREATE"
	nsexecOpEnter   string = "ENTER"
	nsexecNSTypeKey string = "NS_TYPE"
	nsexecNSPathKey string = "NS_PATH"
)

type arg struct {
	key   string
	value string
}

func newNamespaceHelper(op NamespaceOpType, t NamespaceType, args ...arg) (*namespaceHelper, error) {
	if op != NamespaceOpCreate && op != NamespaceOpRelease {
		return nil, errors.New("Invalid namespaceOpType")
	}
	cmd := exec.Command("/proc/self/exe")
	cmd.Args = append(cmd.Args, string(op), "--type", string(t))
	for _, a := range args {
		cmd.Args = append(cmd.Args, a.key)
		if a.value != "" {
			cmd.Args = append(cmd.Args, a.value)
		}
	}
	switch op {
	case NamespaceOpCreate:
		cmd.Env = append(
			cmd.Env,
			nsexecOpKey+"="+nsexecOpCreate,
			nsexecNSTypeKey+"="+string(t),
		)
	case NamespaceOpRelease:
		{
			ok := false
			for _, arg := range args {
				if arg.key == "--pid" {
					ok = true
					cmd.Env = append(
						cmd.Env,
						nsexecOpKey+"="+nsexecOpEnter,
						nsexecNSPathKey+"="+fmt.Sprintf("/proc/%s/ns/%s", arg.value, string(t)),
					)
					break
				}
			}
		}
	default:
		panic("Invalid NamespaceOpType")
	}
	return &namespaceHelper{
		cmd:     cmd,
		newNSFd: -1,
		t:       t,
		op:      op,
	}, nil
}

func (helper *namespaceHelper) do() error {
	stdin, err := helper.cmd.StdinPipe()
	if err != nil {
		return errors.Wrap(err, "Can't get stdin from cmd")
	}
	stdout, err := helper.cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "Can't get stdout from cmd")
	}
	reader := bufio.NewReader(stdout)
	err = helper.cmd.Start()
	if err != nil {
		return errors.Wrap(err, "Can't start cmd")
	}
	defer helper.cmd.Wait()
	ret, err := reader.ReadString('\n')
	if err != nil {
		return errors.Wrap(err, "Can't read ret value from cmd")
	}
	var msg string
	if strings.HasPrefix(ret, "error") {
		fmt.Sscanf(ret, NamespaceErrorFormat, &msg)
		return errors.Errorf("Failed to execute cmd, error %s", msg)
	}
	if helper.op == NamespaceOpCreate {
		_, err = fmt.Sscanf(ret, NamespaceReturnFormat, &msg)
		if err != nil {
			return errors.Wrapf(err, "Invalid return format %s", ret)
		}
		pid, err := strconv.Atoi(msg)
		if err != nil {
			return errors.Errorf("Invalid return format %s", msg)
		}
		if pid != helper.cmd.Process.Pid {
			return errors.Errorf("Pid didn't match %d %d", pid, helper.cmd.Process.Pid)
		}
		fd, err := OpenNSFd(helper.t, pid)
		if err != nil {
			return errors.Wrap(err, "Can't open ns file")
		}
		helper.newNSFd = fd
		// tell child process that we are all good
		io.WriteString(stdin, "OK")
	}
	return nil
}

func (helper *namespaceHelper) getFd() int {
	return helper.newNSFd
}
