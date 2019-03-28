package statuspage

var logger *loggerWrapper

type Logger interface{
	Error(error)
}

type loggerWrapper struct {
	Logger
}

func SetLogger(newLogger Logger) {
	logger = &loggerWrapper{Logger: newLogger}
	return
}

func (l *loggerWrapper) IfError(err error) {
	if l == nil {
		return
	}
	if err == nil {
		return
	}
	if l.Logger == nil {
		return
	}
	l.Logger.Error(err)
}
