package statuspage

var logger *loggerWrapper

// Logger is an interface of a logger that could be set via function `SetLogger` as the logger of this package.
// If there will be any errors then they will be written via the logger.
type Logger interface {
	Error(error)
}

type loggerWrapper struct {
	Logger
}

// SetLogger sets the logger to be used to report about errors
func SetLogger(newLogger Logger) {
	logger = &loggerWrapper{Logger: newLogger}
	return
}

// IfError prints the error "err" via the logger if err != nil
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
