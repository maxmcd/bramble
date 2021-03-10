package logger

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

func newDebugLogger() *zap.SugaredLogger {
	cfg := zap.NewDevelopmentConfig()
	cfg.Encoding = "conditional"
	_ = zap.RegisterEncoder("conditional", newModuleEncoder)
	cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	logger, _ := cfg.Build()
	return logger.Sugar()
}

func init() {
	// BRAMBLE_LOG=level=module
	os.Getenv("BRAMBLE_LOG")
}

var (
	Logger = newDebugLogger()
	Debugw = Logger.Debugw
	Debug  = Logger.Debug
	Info   = Logger.Info
)

func resetGlobals() {
	Debugw = Logger.Debugw
	Debug = Logger.Debug
	Info = Logger.Info
}

func SetDebugLogger() {
	Logger = newDebugLogger()
	resetGlobals()
}

func newModuleEncoder(cfg zapcore.EncoderConfig) (zapcore.Encoder, error) {
	me := moduleEncoder{
		Encoder: zapcore.NewJSONEncoder(cfg),
		level:   zapcore.ErrorLevel,
		modules: map[string]zapcore.Level{},
	}
	val := os.Getenv("BRAMBLE_LOG")
	if val == "" {
		return me, nil
	}
	lower := strings.ToLower(val)

	stringToLevel := func(str string) (zapcore.Level, bool) {
		for _, lvl := range []zapcore.Level{
			zapcore.DebugLevel,
			zapcore.InfoLevel,
			zapcore.WarnLevel,
			zapcore.ErrorLevel,
			zapcore.PanicLevel,
			zapcore.FatalLevel} {
			if str == lvl.String() {
				return lvl, true
			}
		}
		if str == "off" {
			return zapcore.DebugLevel - 1, true
		}
		return 0, false
	}

	// "error,hello=warn"
	for _, match := range strings.Split(lower, ",") {
		lvl, found := stringToLevel(match)
		switch {
		case found: // this is just a level
			me.level = lvl
		case !strings.Contains(match, "="): // no equal and no level, so just a module name
			me.modules[match] = zapcore.DebugLevel
		default: // it's the module=level syntax, ignore if malformed
			parts := strings.Split(match, "=")
			if len(parts) == 2 {
				module, lvlString := parts[0], parts[1]
				if lvl, found := stringToLevel(lvlString); found {
					me.modules[module] = lvl
				}
			}
		}
	}
	return me, nil
}

type moduleEncoder struct {
	zapcore.Encoder
	level   zapcore.Level
	modules map[string]zapcore.Level
}

func (me moduleEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	line, err := me.Encoder.EncodeEntry(entry, fields)
	effectiveLevel := me.level
	moduleWithFileAndLine := entry.Caller.TrimmedPath()
	if moduleWithFileAndLine != "undefined" {
		idx := strings.IndexRune(moduleWithFileAndLine, '/')
		if idx > 0 {
			moduleName := moduleWithFileAndLine[:idx]
			if lvl, found := me.modules[moduleName]; found {
				effectiveLevel = lvl
			}
		}
	}
	if entry.Level < effectiveLevel {
		line.Reset()
		return line, err
	}
	return line, err
}

func newInfoLogger() *zap.SugaredLogger {
	cfg := zap.NewDevelopmentConfig()
	cfg.Encoding = "conditional"
	_ = zap.RegisterEncoder("conditional", newModuleEncoder)
	cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, _ := cfg.Build()
	return logger.Sugar()
}

func SetInfoLogger() {
	Logger = newInfoLogger()
	resetGlobals()
}

func Print(a ...interface{}) {
	fmt.Println(a...)
}
func Printfln(format string, a ...interface{}) {
	fmt.Printf(format+"\n", a...)
}
