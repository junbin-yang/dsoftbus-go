package logger

import (
	"fmt"
	"io"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Level = zapcore.Level

const (
	DebugLevel = zapcore.DebugLevel
	InfoLevel  = zapcore.InfoLevel
	WarnLevel  = zapcore.WarnLevel
	ErrorLevel = zapcore.ErrorLevel
	PanicLevel = zapcore.PanicLevel
	FatalLevel = zapcore.FatalLevel
)

type Logger struct {
	l  *zap.Logger
	al *zap.AtomicLevel
}

func New(out io.Writer, level Level, opts ...Option) *Logger {
	if out == nil {
		out = os.Stderr
	}

	al := zap.NewAtomicLevelAt(level)

	core := zapcore.NewCore(
		GetEncoder(),
		zapcore.AddSync(out),
		al,
	)
	return &Logger{l: zap.New(core, opts...), al: &al}
}

// 自定义Encoder
func GetEncoder() zapcore.Encoder {
	return zapcore.NewConsoleEncoder(
		zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller_line",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding, // 默认换行符"\n"
			EncodeLevel:    cEncodeLevel,
			EncodeTime:     cEncodeTime,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   cEncodeCaller,
		})
}

// 自定义日志级别显示
func cEncodeLevel(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString("[" + level.CapitalString() + "]")
}

// 自定义时间格式显示
func cEncodeTime(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	var logTmFmt = "2006-01-02 15:04:05"
	enc.AppendString("[" + t.Format(logTmFmt) + "]")
}

// 自定义行号显示
func cEncodeCaller(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString("[" + caller.TrimmedPath() + "]")
}

func (l *Logger) SetLevel(level Level) {
	if l.al != nil {
		l.al.SetLevel(level)
	}
}

type Field = zap.Field

func (l *Logger) Debug(msg string, fields ...Field) {
	l.l.Debug(msg, fields...)
}

func (l *Logger) Info(msg string, fields ...Field) {
	l.l.Info(msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...Field) {
	l.l.Warn(msg, fields...)
}

func (l *Logger) Error(msg string, fields ...Field) {
	l.l.Error(msg, fields...)
}

func (l *Logger) Panic(msg string, fields ...Field) {
	l.l.Panic(msg, fields...)
}

func (l *Logger) Fatal(msg string, fields ...Field) {
	l.l.Fatal(msg, fields...)
}

func (l *Logger) Sync() error {
	return l.l.Sync()
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.Debug(msg)
}

func (l *Logger) Infof(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.Info(msg)
}

func (l *Logger) Warnf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.Warn(msg)
}

func (l *Logger) Errorf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.Error(msg)
}

func (l *Logger) Panicf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.Panic(msg)
}

func (l *Logger) Fatalf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	l.Fatal(msg)
}

var std = New(os.Stderr, InfoLevel, AddCaller(), AddCallerSkip(2))

func Default() *Logger         { return std }
func ReplaceDefault(l *Logger) { std = l }

func SetLevel(level Level) { std.SetLevel(level) }

func Debug(msg string, fields ...Field) { std.Debug(msg, fields...) }
func Info(msg string, fields ...Field)  { std.Info(msg, fields...) }
func Warn(msg string, fields ...Field)  { std.Warn(msg, fields...) }
func Error(msg string, fields ...Field) { std.Error(msg, fields...) }
func Panic(msg string, fields ...Field) { std.Panic(msg, fields...) }
func Fatal(msg string, fields ...Field) { std.Fatal(msg, fields...) }

func Debugf(format string, v ...interface{}) { std.Debugf(format, v...) }
func Infof(format string, v ...interface{})  { std.Infof(format, v...) }
func Warnf(format string, v ...interface{})  { std.Warnf(format, v...) }
func Errorf(format string, v ...interface{}) { std.Errorf(format, v...) }
func Panicf(format string, v ...interface{}) { std.Panicf(format, v...) }
func Fatalf(format string, v ...interface{}) { std.Fatalf(format, v...) }

func Sync() error { return std.Sync() }

func GetError(e error) Field {
	return zap.Error(e)
}
