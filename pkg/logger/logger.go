package logger

import (
	"fmt"
	"log"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger 定义了一个通用的日志接口
type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Warnf(template string, args ...interface{})
	Errorf(template string, args ...interface{})
	Fatalf(template string, args ...interface{})
}

// Config 包含了日志系统的配置
type Config struct {
	Type  string // "zap" or "default"
	Level string // "debug", "info", "warn", "error"
}

// New 根据配置创建一个新的 Logger 实例
func New(config Config) (Logger, error) {
	switch strings.ToLower(config.Type) {
	case "zap":
		return newZapLogger(config.Level)
	case "default":
		return newDefaultLogger(), nil
	default:
		return nil, fmt.Errorf("不支持的日志类型: %s", config.Type)
	}
}

// --- Zap Logger 实现 ---

// ZapLogger 是 Logger 接口的 zap 实现
type ZapLogger struct {
	sugaredLogger *zap.SugaredLogger
	rawLogger     *zap.Logger
}

func newZapLogger(level string) (*ZapLogger, error) {
	zapLevel := zapcore.InfoLevel
	switch strings.ToLower(level) {
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

	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.Lock(os.Stdout),
		zapLevel,
	))

	return &ZapLogger{
		sugaredLogger: logger.Sugar(),
		rawLogger:     logger,
	}, nil
}

// Raw 返回底层的 *zap.Logger，用于需要特定实现的场景（如 fiberzap）
func (l *ZapLogger) Raw() *zap.Logger {
	return l.rawLogger
}

func (l *ZapLogger) Debug(args ...interface{}) { l.sugaredLogger.Debug(args...) }
func (l *ZapLogger) Info(args ...interface{})  { l.sugaredLogger.Info(args...) }
func (l *ZapLogger) Warn(args ...interface{})  { l.sugaredLogger.Warn(args...) }
func (l *ZapLogger) Error(args ...interface{}) { l.sugaredLogger.Error(args...) }
func (l *ZapLogger) Fatal(args ...interface{}) { l.sugaredLogger.Fatal(args...) }
func (l *ZapLogger) Debugf(template string, args ...interface{}) {
	l.sugaredLogger.Debugf(template, args...)
}
func (l *ZapLogger) Infof(template string, args ...interface{}) {
	l.sugaredLogger.Infof(template, args...)
}
func (l *ZapLogger) Warnf(template string, args ...interface{}) {
	l.sugaredLogger.Warnf(template, args...)
}
func (l *ZapLogger) Errorf(template string, args ...interface{}) {
	l.sugaredLogger.Errorf(template, args...)
}
func (l *ZapLogger) Fatalf(template string, args ...interface{}) {
	l.sugaredLogger.Fatalf(template, args...)
}

// --- Standard Log 实现 ---

// DefaultLogger 是 Logger 接口的标准库实现
type DefaultLogger struct {
	logger *log.Logger
}

func newDefaultLogger() *DefaultLogger {
	return &DefaultLogger{
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

func (l *DefaultLogger) Debug(args ...interface{}) {
	l.logger.Println(append([]interface{}{"[DEBUG]"}, args...)...)
}
func (l *DefaultLogger) Info(args ...interface{}) {
	l.logger.Println(append([]interface{}{"[INFO]"}, args...)...)
}
func (l *DefaultLogger) Warn(args ...interface{}) {
	l.logger.Println(append([]interface{}{"[WARN]"}, args...)...)
}
func (l *DefaultLogger) Error(args ...interface{}) {
	l.logger.Println(append([]interface{}{"[ERROR]"}, args...)...)
}
func (l *DefaultLogger) Fatal(args ...interface{}) {
	l.logger.Fatal(append([]interface{}{"[FATAL]"}, args...)...)
}
func (l *DefaultLogger) Debugf(template string, args ...interface{}) {
	l.printf("[DEBUG]", template, args...)
}
func (l *DefaultLogger) Infof(template string, args ...interface{}) {
	l.printf("[INFO]", template, args...)
}
func (l *DefaultLogger) Warnf(template string, args ...interface{}) {
	l.printf("[WARN]", template, args...)
}
func (l *DefaultLogger) Errorf(template string, args ...interface{}) {
	l.printf("[ERROR]", template, args...)
}
func (l *DefaultLogger) Fatalf(template string, args ...interface{}) {
	l.fatalf("[FATAL]", template, args...)
}

func (l *DefaultLogger) printf(prefix, template string, args ...interface{}) {
	l.logger.Printf(prefix+" "+template, args...)
}
func (l *DefaultLogger) fatalf(prefix, template string, args ...interface{}) {
	l.logger.Fatalf(prefix+" "+template, args...)
}
