package logger

import (
	"context"
	fiberlog "github.com/gofiber/fiber/v3/log"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	globalLogger Logger = newDefaultLogger()
	globalConfig Config
	mu           sync.RWMutex
)

type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Panic(args ...interface{})
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Warnf(template string, args ...interface{})
	Errorf(template string, args ...interface{})
	Fatalf(template string, args ...interface{})
	Panicf(template string, args ...interface{})
	Debugw(msg string, keysAndValues ...interface{})
	Infow(msg string, keysAndValues ...interface{})
	Warnw(msg string, keysAndValues ...interface{})
	Errorw(msg string, keysAndValues ...interface{})
	Fatalw(msg string, keysAndValues ...interface{})
	Panicw(msg string, keysAndValues ...interface{})
	Raw() *zap.Logger
}

type Config struct {
	Type   string
	Level  string
	File   FileConfig
	Output io.Writer
}

type FileConfig struct {
	Enabled    bool
	Path       string
	MaxSize    int
	MaxAge     int
	MaxBackups int
	Compress   bool
}

func Init(config Config) {
	mu.Lock()
	defer mu.Unlock()

	globalConfig = config

	var err error
	switch strings.ToLower(config.Type) {
	case "zap":
		globalLogger, err = newZapLogger(config)
		if err != nil {
			log.Fatalf("无法初始化 Zap logger: %v", err)
		}
	case "default":
		globalLogger = newDefaultLogger()
	default:
		log.Fatalf("不支持的日志类型: %s", config.Type)
	}
}

func Debug(args ...interface{}) { mu.RLock(); defer mu.RUnlock(); globalLogger.Debug(args...) }
func Info(args ...interface{})  { mu.RLock(); defer mu.RUnlock(); globalLogger.Info(args...) }
func Warn(args ...interface{})  { mu.RLock(); defer mu.RUnlock(); globalLogger.Warn(args...) }
func Error(args ...interface{}) { mu.RLock(); defer mu.RUnlock(); globalLogger.Error(args...) }
func Fatal(args ...interface{}) { mu.RLock(); defer mu.RUnlock(); globalLogger.Fatal(args...) }
func Panic(args ...interface{}) { mu.RLock(); defer mu.RUnlock(); globalLogger.Panic(args...) }
func Debugf(template string, args ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Debugf(template, args...)
}
func Infof(template string, args ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Infof(template, args...)
}
func Warnf(template string, args ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Warnf(template, args...)
}
func Errorf(template string, args ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Errorf(template, args...)
}
func Fatalf(template string, args ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Fatalf(template, args...)
}
func Panicf(template string, args ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Panicf(template, args...)
}
func Debugw(msg string, keysAndValues ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Debugw(msg, keysAndValues...)
}
func Infow(msg string, keysAndValues ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Infow(msg, keysAndValues...)
}
func Warnw(msg string, keysAndValues ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Warnw(msg, keysAndValues...)
}
func Errorw(msg string, keysAndValues ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Errorw(msg, keysAndValues...)
}
func Fatalw(msg string, keysAndValues ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Fatalw(msg, keysAndValues...)
}
func Panicw(msg string, keysAndValues ...interface{}) {
	mu.RLock()
	defer mu.RUnlock()
	globalLogger.Panicw(msg, keysAndValues...)
}
func Raw() *zap.Logger { mu.RLock(); defer mu.RUnlock(); return globalLogger.Raw() }

type zapLogger struct {
	sugaredLogger *zap.SugaredLogger
	rawLogger     *zap.Logger
}

func newZapLogger(config Config) (Logger, error) {
	zapLevel := zapcore.InfoLevel
	switch strings.ToLower(config.Level) {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	}
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	consoleWriter := zapcore.Lock(os.Stdout)
	if config.Output != nil {
		consoleWriter = zapcore.AddSync(config.Output)
	}

	consoleCore := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), consoleWriter, zapLevel)
	cores := []zapcore.Core{consoleCore}

	if config.File.Enabled {
		fileWriter := zapcore.AddSync(&lumberjack.Logger{Filename: config.File.Path, MaxSize: config.File.MaxSize, MaxBackups: config.File.MaxBackups, MaxAge: config.File.MaxAge, Compress: config.File.Compress})
		fileCore := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), fileWriter, zapLevel)
		cores = append(cores, fileCore)
	}
	logger := zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.AddCallerSkip(2))
	return &zapLogger{sugaredLogger: logger.Sugar(), rawLogger: logger}, nil
}

func (l *zapLogger) Raw() *zap.Logger          { return l.rawLogger }
func (l *zapLogger) Debug(args ...interface{}) { l.sugaredLogger.Debug(args...) }
func (l *zapLogger) Info(args ...interface{})  { l.sugaredLogger.Info(args...) }
func (l *zapLogger) Warn(args ...interface{})  { l.sugaredLogger.Warn(args...) }
func (l *zapLogger) Error(args ...interface{}) { l.sugaredLogger.Error(args...) }
func (l *zapLogger) Fatal(args ...interface{}) { l.sugaredLogger.Fatal(args...) }
func (l *zapLogger) Panic(args ...interface{}) { l.sugaredLogger.Panic(args...) }
func (l *zapLogger) Debugf(template string, args ...interface{}) {
	l.sugaredLogger.Debugf(template, args...)
}
func (l *zapLogger) Infof(template string, args ...interface{}) {
	l.sugaredLogger.Infof(template, args...)
}
func (l *zapLogger) Warnf(template string, args ...interface{}) {
	l.sugaredLogger.Warnf(template, args...)
}
func (l *zapLogger) Errorf(template string, args ...interface{}) {
	l.sugaredLogger.Errorf(template, args...)
}
func (l *zapLogger) Fatalf(template string, args ...interface{}) {
	l.sugaredLogger.Fatalf(template, args...)
}
func (l *zapLogger) Panicf(template string, args ...interface{}) {
	l.sugaredLogger.Panicf(template, args...)
}
func (l *zapLogger) Debugw(msg string, keysAndValues ...interface{}) {
	l.sugaredLogger.Debugw(msg, keysAndValues...)
}
func (l *zapLogger) Infow(msg string, keysAndValues ...interface{}) {
	l.sugaredLogger.Infow(msg, keysAndValues...)
}
func (l *zapLogger) Warnw(msg string, keysAndValues ...interface{}) {
	l.sugaredLogger.Warnw(msg, keysAndValues...)
}
func (l *zapLogger) Errorw(msg string, keysAndValues ...interface{}) {
	l.sugaredLogger.Errorw(msg, keysAndValues...)
}
func (l *zapLogger) Fatalw(msg string, keysAndValues ...interface{}) {
	l.sugaredLogger.Fatalw(msg, keysAndValues...)
}
func (l *zapLogger) Panicw(msg string, keysAndValues ...interface{}) {
	l.sugaredLogger.Panicw(msg, keysAndValues...)
}

type defaultLogger struct {
	logger *log.Logger
}

func newDefaultLogger() *defaultLogger {
	return &defaultLogger{logger: log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)}
}

func (l *defaultLogger) Raw() *zap.Logger { return nil }
func (l *defaultLogger) Debug(args ...interface{}) {
	l.logger.Println(append([]interface{}{"[DEBUG]"}, args...)...)
}
func (l *defaultLogger) Info(args ...interface{}) {
	l.logger.Println(append([]interface{}{"[INFO]"}, args...)...)
}
func (l *defaultLogger) Warn(args ...interface{}) {
	l.logger.Println(append([]interface{}{"[WARN]"}, args...)...)
}
func (l *defaultLogger) Error(args ...interface{}) {
	l.logger.Println(append([]interface{}{"[ERROR]"}, args...)...)
}
func (l *defaultLogger) Fatal(args ...interface{}) {
	l.logger.Fatal(append([]interface{}{"[FATAL]"}, args...)...)
}
func (l *defaultLogger) Panic(args ...interface{}) {
	l.logger.Panic(append([]interface{}{"[PANIC]"}, args...)...)
}
func (l *defaultLogger) Debugf(template string, args ...interface{}) {
	l.printf("[DEBUG]", template, args...)
}
func (l *defaultLogger) Infof(template string, args ...interface{}) {
	l.printf("[INFO]", template, args...)
}
func (l *defaultLogger) Warnf(template string, args ...interface{}) {
	l.printf("[WARN]", template, args...)
}
func (l *defaultLogger) Errorf(template string, args ...interface{}) {
	l.printf("[ERROR]", template, args...)
}
func (l *defaultLogger) Fatalf(template string, args ...interface{}) {
	l.fatalf("[FATAL]", template, args...)
}
func (l *defaultLogger) Panicf(template string, args ...interface{}) {
	l.fatalf("[PANIC]", template, args...)
}
func (l *defaultLogger) printf(prefix, template string, args ...interface{}) {
	l.logger.Printf(prefix+" "+template, args...)
}
func (l *defaultLogger) fatalf(prefix, template string, args ...interface{}) {
	l.logger.Fatalf(prefix+" "+template, args...)
}
func (l *defaultLogger) Debugw(msg string, keysAndValues ...interface{}) {
	l.logger.Printf("[DEBUG] %s %v", msg, keysAndValues)
}
func (l *defaultLogger) Infow(msg string, keysAndValues ...interface{}) {
	l.logger.Printf("[INFO] %s %v", msg, keysAndValues)
}
func (l *defaultLogger) Warnw(msg string, keysAndValues ...interface{}) {
	l.logger.Printf("[WARN] %s %v", msg, keysAndValues)
}
func (l *defaultLogger) Errorw(msg string, keysAndValues ...interface{}) {
	l.logger.Printf("[ERROR] %s %v", msg, keysAndValues)
}
func (l *defaultLogger) Fatalw(msg string, keysAndValues ...interface{}) {
	l.logger.Fatalf("[FATAL] %s %v", msg, keysAndValues)
}
func (l *defaultLogger) Panicw(msg string, keysAndValues ...interface{}) {
	l.logger.Panicf("[PANIC] %s %v", msg, keysAndValues)
}

type LogWriter struct{}

func (lw *LogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		Info(msg)
	}
	return len(p), nil
}

var _ fiberlog.AllLogger[any] = (*FiberLogAdapter)(nil)

type FiberLogAdapter struct{}

func NewFiberLogAdapter() fiberlog.AllLogger[any] {
	return &FiberLogAdapter{}
}

func (a *FiberLogAdapter) Trace(v ...interface{})                                { Debug(v...) }
func (a *FiberLogAdapter) Debug(v ...interface{})                                { Debug(v...) }
func (a *FiberLogAdapter) Info(v ...interface{})                                 { Info(v...) }
func (a *FiberLogAdapter) Warn(v ...interface{})                                 { Warn(v...) }
func (a *FiberLogAdapter) Error(v ...interface{})                                { Error(v...) }
func (a *FiberLogAdapter) Fatal(v ...interface{})                                { Fatal(v...) }
func (a *FiberLogAdapter) Panic(v ...interface{})                                { Panic(v...) }
func (a *FiberLogAdapter) Tracef(format string, v ...interface{})                { Debugf(format, v...) }
func (a *FiberLogAdapter) Debugf(format string, v ...interface{})                { Debugf(format, v...) }
func (a *FiberLogAdapter) Infof(format string, v ...interface{})                 { Infof(format, v...) }
func (a *FiberLogAdapter) Warnf(format string, v ...interface{})                 { Warnf(format, v...) }
func (a *FiberLogAdapter) Errorf(format string, v ...interface{})                { Errorf(format, v...) }
func (a *FiberLogAdapter) Fatalf(format string, v ...interface{})                { Fatalf(format, v...) }
func (a *FiberLogAdapter) Panicf(format string, v ...interface{})                { Panicf(format, v...) }
func (a *FiberLogAdapter) Tracew(msg string, keysAndValues ...any)               { Debugw(msg, keysAndValues...) }
func (a *FiberLogAdapter) Debugw(msg string, keysAndValues ...any)               { Debugw(msg, keysAndValues...) }
func (a *FiberLogAdapter) Infow(msg string, keysAndValues ...any)                { Infow(msg, keysAndValues...) }
func (a *FiberLogAdapter) Warnw(msg string, keysAndValues ...any)                { Warnw(msg, keysAndValues...) }
func (a *FiberLogAdapter) Errorw(msg string, keysAndValues ...any)               { Errorw(msg, keysAndValues...) }
func (a *FiberLogAdapter) Fatalw(msg string, keysAndValues ...any)               { Fatalw(msg, keysAndValues...) }
func (a *FiberLogAdapter) Panicw(msg string, keysAndValues ...any)               { Panicw(msg, keysAndValues...) }
func (a *FiberLogAdapter) WithContext(ctx context.Context) fiberlog.CommonLogger { return a }

func (a *FiberLogAdapter) SetLevel(level fiberlog.Level) {
	mu.Lock()
	newConfig := globalConfig
	mu.Unlock()

	switch level {
	case fiberlog.LevelTrace, fiberlog.LevelDebug:
		newConfig.Level = "debug"
	case fiberlog.LevelInfo:
		newConfig.Level = "info"
	case fiberlog.LevelWarn:
		newConfig.Level = "warn"
	case fiberlog.LevelError:
		newConfig.Level = "error"
	default:
		Warnw("SetLevel called with unsupported level for dynamic change", "level", level)
		return
	}
	Init(newConfig)
}

func (a *FiberLogAdapter) SetOutput(w io.Writer) {
	mu.Lock()
	newConfig := globalConfig
	mu.Unlock()

	newConfig.Output = w
	Init(newConfig)
}

func (a *FiberLogAdapter) Logger() any {
	return Raw()
}
