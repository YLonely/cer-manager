package namespace

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type namespaceHelper struct {
	newNSFile *os.File
	cmd       *exec.Cmd
	t         NamespaceType
	op        NamespaceOpType
}

const (
	nsexecOpKey     string = "__OP_TYPE__"
	nsexecOpCreate  string = "CREATE"
	nsexecOpEnter   string = "ENTER"
	nsexecNSTypeKey string = "__NS_TYPE__"
	nsexecNSPathKey string = "__NS_PATH__"
)

type arg struct {
	key   string
	value string
}

func newNamespaceCreateHelper(t NamespaceType, src, bundle string) (*namespaceHelper, error) {
	cmd := exec.Command("/proc/self/exe", "nsexec", "create")
	if t == MNT {
		cmd.Args = append(cmd.Args, "--src", src, "--bundle", bundle)
	}
	cmd.Args = append(cmd.Args, string(t))
	cmd.Env = append(
		cmd.Env,
		nsexecOpKey+"="+nsexecOpCreate,
		nsexecNSTypeKey+"="+string(t),
	)
	return &namespaceHelper{
		cmd: cmd,
		t:   t,
		op:  NamespaceOpCreate,
	}, nil
}

func newNamespaceReleaseHelper(t NamespaceType, pid int, fd int, bundle string) (*namespaceHelper, error) {
	cmd := exec.Command("/proc/self/exe", "nsexec", "release")
	if t == MNT {
		cmd.Args = append(cmd.Args, "--bundle", bundle)
	}
	cmd.Args = append(cmd.Args, string(t))
	cmd.Env = append(
		cmd.Env,
		nsexecOpKey+"="+nsexecOpEnter,
		nsexecNSTypeKey+"="+string(t),
		nsexecNSPathKey+"="+fmt.Sprintf("/proc/%d/fd/%d", pid, fd),
	)
	return &namespaceHelper{
		cmd: cmd,
		t:   t,
		op:  NamespaceOpRelease,
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
		f, err := OpenNSFile(helper.t, pid)
		if err != nil {
			return errors.Wrap(err, "Can't open ns file")
		}
		helper.newNSFile = f
		// tell child process that it can exits
		io.WriteString(stdin, "OK\n")
	}
	return nil
}

func (helper *namespaceHelper) nsFile() *os.File {
	return helper.newNSFile
}
