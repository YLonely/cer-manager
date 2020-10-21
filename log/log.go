package log

import (
	"github.com/YLonely/cr-daemon/service"
	"github.com/sirupsen/logrus"
)

var loggers = map[service.ServiceType]*logrus.Entry{}

func Logger(t service.ServiceType) *logrus.Entry {
	var ret *logrus.Entry
	if logger, exists := loggers[t]; !exists {
		loggers[t] = logrus.WithField("service", t)
		ret = loggers[t]
	} else {
		ret = logger
	}
	return ret
}
