package logger

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap/zapcore"
)

func Test_newInfoLogger(t *testing.T) {
	logger := newDebugLogger()
	logger.Info("hi")
	logger.Debug("hi")

	for _, match := range strings.Split("foo", ",") {
		strings.Split(match, "=")
	}
}

func Test_newModuleEncoder(t *testing.T) {
	tests := []struct {
		arg  string
		want moduleEncoder
	}{
		{
			arg: "hello",
			want: moduleEncoder{
				level: zapcore.ErrorLevel,
				modules: map[string]zapcore.Level{
					"hello": zapcore.DebugLevel,
				},
			},
		}, {
			arg:  "warn",
			want: moduleEncoder{level: zapcore.WarnLevel},
		}, {
			arg:  "WARN",
			want: moduleEncoder{level: zapcore.WarnLevel},
		}, {
			arg: "hello=debug",
			want: moduleEncoder{
				level: zapcore.ErrorLevel,
				modules: map[string]zapcore.Level{
					"hello": zapcore.DebugLevel,
				},
			},
		}, {
			arg:  "off",
			want: moduleEncoder{level: zapcore.DebugLevel - 1},
		}, {
			arg:  "OFF",
			want: moduleEncoder{level: zapcore.DebugLevel - 1},
		}, {
			arg: "hello,std::option",
			want: moduleEncoder{
				level: zapcore.ErrorLevel,
				modules: map[string]zapcore.Level{
					"hello": zapcore.DebugLevel,
					// TODO: pkg paths in go
					"std::option": zapcore.DebugLevel,
				},
			},
		}, {
			arg: "info,hello=warn",
			want: moduleEncoder{
				level: zapcore.InfoLevel,
				modules: map[string]zapcore.Level{
					"hello": zapcore.WarnLevel,
				},
			},
		}, {
			arg: "error,hello=off",
			want: moduleEncoder{
				level: zapcore.ErrorLevel,
				modules: map[string]zapcore.Level{
					"hello": zapcore.DebugLevel - 1,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			os.Setenv("BRAMBLE_LOG", tt.arg)
			got, _ := newModuleEncoder(zapcore.EncoderConfig{})
			me := got.(moduleEncoder)
			meStripped := moduleEncoder{
				level:   me.level,
				modules: me.modules,
			}
			// make deepequal happy since we don't allocate the empty map
			// in the test cases
			if tt.want.modules == nil && len(meStripped.modules) == 0 {
				meStripped.modules = nil
			}
			if !reflect.DeepEqual(meStripped, tt.want) {
				t.Errorf("newModuleEncoder() = %v, want %v", meStripped, tt.want)
			}
		})
	}
}
