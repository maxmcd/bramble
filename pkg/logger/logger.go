package logger

import (
	"fmt"

	"go.uber.org/zap"
)

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

func Print(a ...interface{}) {
	fmt.Println(a...)
}
func Printfln(format string, a ...interface{}) {
	fmt.Printf(format+"\n", a...)
}
