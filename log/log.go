package log

import (
	"encoding/json"
	"strings"
	"time"

	cerm "github.com/YLonely/cer-manager"
	"github.com/sirupsen/logrus"
)

func init() {
	l, _ := time.LoadLocation("Asia/Chongqing")
	time.Local = l
	logrus.SetFormatter(
		formatter{
			&logrus.TextFormatter{
				FullTimestamp: true,
			},
		},
	)
}

type formatter struct {
	logrus.Formatter
}

func (f formatter) Format(e *logrus.Entry) ([]byte, error) {
	e.Time = e.Time.Local()
	return f.Formatter.Format(e)
}

type logItem struct {
	stype  cerm.ServiceType
	method string
}

var loggers = map[logItem]*logrus.Entry{}

func Logger(t cerm.ServiceType, method string) *logrus.Entry {
	var ret *logrus.Entry
	var serviceStr string
	var exists bool
	if method == "" {
		method = "Unknown"
	}
	item := logItem{
		stype:  t,
		method: method,
	}
	if serviceStr, exists = cerm.Type2Services[t]; !exists {
		serviceStr = "unknown"
	}
	if logger, exists := loggers[item]; !exists {
		loggers[item] = logrus.WithFields(logrus.Fields{
			"service": serviceStr,
			"method":  method,
		})
		ret = loggers[item]
	} else {
		ret = logger
	}
	return ret
}

func WithInterface(entry *logrus.Entry, key string, value interface{}) *logrus.Entry {
	valueJSON, _ := json.Marshal(value)
	str := strings.ReplaceAll(string(valueJSON), "\"", "")
	return entry.WithField(key, str)
}
