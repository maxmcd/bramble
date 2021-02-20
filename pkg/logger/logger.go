package logger

import "go.uber.org/zap"

func newSugaredLogger() *zap.SugaredLogger {
	logger, _ := zap.NewDevelopment()
	return logger.Sugar()
}

var Logger = newSugaredLogger()
var Debugw = Logger.Debugw
