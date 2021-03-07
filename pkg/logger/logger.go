package logger

import (
	"fmt"

	"go.uber.org/zap"
)

func newDebugLogger() *zap.SugaredLogger {
	logger, _ := zap.NewDevelopment()
	return logger.Sugar()
}

var (
	Logger = newInfoLogger()
	Debugw = Logger.Debugw
	Debug  = Logger.Debug
	Info   = Logger.Info
)

func SetDebugLogger() {
	Logger = newDebugLogger()
}

func newInfoLogger() *zap.SugaredLogger {
	cfg := zap.NewDevelopmentConfig()
	cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, _ := cfg.Build()
	return logger.Sugar()
}

func SetInfoLogger() {
	Logger = newInfoLogger()
}

func Print(a ...interface{}) {
	fmt.Println(a...)
}
func Printfln(format string, a ...interface{}) {
	fmt.Printf(format+"\n", a...)
}
