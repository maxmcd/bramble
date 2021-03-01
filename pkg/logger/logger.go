package logger

import "go.uber.org/zap"

func newSugaredLogger() *zap.SugaredLogger {
	logger, _ := zap.NewDevelopment()
	return logger.Sugar()
}

var (
	Logger = newSugaredLogger()
	Debugw = Logger.Debugw
	Debug  = Logger.Debug
	Info   = Logger.Info
)
