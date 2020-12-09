package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
)

const (
	sysPath = "/proc/sys"
)

// SysCtlRead reads value from /proc/sys/${item}
func SysCtlRead(item string) (string, error) {
	filePath := path.Join(sysPath, item)
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", errors.Wrap(err, "failed to read file "+filePath)
	}
	valueStr := strings.Trim(string(content), " \n\t")
	return valueStr, nil
}

// SysCtlWrite writes a value to /proc/sys/${item}
func SysCtlWrite(item string, value string) error {
	filePath := path.Join(sysPath, item)
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "%s\n", value); err != nil {
		return err
	}
	return nil
}
