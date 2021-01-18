package log

import (
	"encoding/json"
	"strings"
	"time"

	cerm "github.com/YLonely/cer-manager"
	"github.com/sirupsen/logrus"
)

type Level int

const (
	LevelDebug       = Level(logrus.DebugLevel)
	LevelInfo        = Level(logrus.InfoLevel)
	LevelWarn        = Level(logrus.WarnLevel)
	LevelError       = Level(logrus.ErrorLevel)
	rfc3339NanoFixed = "2006-01-02T15:04:05.000000"
)

func init() {
	l, _ := time.LoadLocation("Asia/Chongqing")
	time.Local = l
	logrus.SetFormatter(
		formatter{
			&logrus.TextFormatter{
				FullTimestamp:   true,
				TimestampFormat: rfc3339NanoFixed,
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

func Logger(t cerm.ServiceType, method string) *logrus.Entry {
	var serviceStr string
	var exists bool
	if method == "" {
		method = "Unknown"
	}
	if serviceStr, exists = cerm.Type2Services[t]; !exists {
		serviceStr = "unknown"
	}
	return logrus.WithFields(logrus.Fields{
		"service": serviceStr,
		"method":  method,
	})
}

func Raw() *logrus.Logger {
	return logrus.StandardLogger()
}

func SetLevel(l Level) {
	logrus.SetLevel(logrus.Level(l))
}

func WithInterface(entry *logrus.Entry, key string, value interface{}) *logrus.Entry {
	valueJSON, _ := json.Marshal(value)
	str := strings.ReplaceAll(string(valueJSON), "\"", "")
	return entry.WithField(key, str)
}
