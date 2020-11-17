package log

import (
	"encoding/json"

	"github.com/YLonely/cer-manager/services"
	"github.com/sirupsen/logrus"
)

type logItem struct {
	stype  services.ServiceType
	method string
}

var loggers = map[logItem]*logrus.Entry{}

func Logger(t services.ServiceType, method string) *logrus.Entry {
	var ret *logrus.Entry
	item := logItem{
		stype:  t,
		method: method,
	}
	if logger, exists := loggers[item]; !exists {
		loggers[item] = logrus.WithFields(logrus.Fields{
			"service": t,
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
	return entry.WithField(key, string(valueJSON))
}
