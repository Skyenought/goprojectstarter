package logger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type GormZapLogger struct {
	zapLogger                 Logger
	logLevel                  gormlogger.LogLevel
	slowThreshold             time.Duration
	ignoreRecordNotFoundError bool
}

// NewGormZapLogger 是 GormZapLogger 的构造函数，将作为 DI provider
func NewGormZapLogger(logger Logger) gormlogger.Interface {
	return &GormZapLogger{
		zapLogger:                 logger,
		logLevel:                  gormlogger.Info, // 默认日志级别
		slowThreshold:             200 * time.Millisecond,
		ignoreRecordNotFoundError: true,
	}
}

func (l *GormZapLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *l
	newLogger.logLevel = level
	return &newLogger
}

func (l *GormZapLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Info {
		l.zapLogger.Infof(msg, data...)
	}
}

func (l *GormZapLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Warn {
		l.zapLogger.Warnf(msg, data...)
	}
}

func (l *GormZapLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Error {
		l.zapLogger.Errorf(msg, data...)
	}
}

func (l *GormZapLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.logLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	msg := "[GORM] %.3fms | %d rows | %s"

	// 处理错误
	if err != nil && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.ignoreRecordNotFoundError) {
		l.zapLogger.Errorf(msg, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		l.zapLogger.Error(err) // 单独打印错误详情
		return
	}

	// 慢查询警告
	if l.slowThreshold > 0 && elapsed > l.slowThreshold {
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.slowThreshold)
		l.zapLogger.Warnf("%s | %s", slowLog, fmt.Sprintf(msg, float64(elapsed.Nanoseconds())/1e6, rows, sql))
		return
	}

	// 普通 SQL 日志
	if l.logLevel >= gormlogger.Info {
		l.zapLogger.Infof(msg, float64(elapsed.Nanoseconds())/1e6, rows, sql)
	}
}
