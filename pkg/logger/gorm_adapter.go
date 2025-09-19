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
	logLevel                  gormlogger.LogLevel
	slowThreshold             time.Duration
	ignoreRecordNotFoundError bool
}

// NewGormZapLogger 不再需要任何参数，因为它将使用全局 logger
func NewGormZapLogger() gormlogger.Interface {
	return &GormZapLogger{
		logLevel:                  gormlogger.Info,
		slowThreshold:             1000 * time.Millisecond,
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
		// 直接调用全局函数
		Infof(msg, data...)
	}
}

func (l *GormZapLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Warn {
		Warnf(msg, data...)
	}
}

func (l *GormZapLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= gormlogger.Error {
		Errorf(msg, data...)
	}
}

func (l *GormZapLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.logLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()
	msg := "[GORM] %.3fms | %d rows | %s"

	if err != nil && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.ignoreRecordNotFoundError) {
		Errorf(msg, float64(elapsed.Nanoseconds())/1e6, rows, sql)
		// 单独打印错误详情
		Error(err)
		return
	}

	if l.slowThreshold > 0 && elapsed > l.slowThreshold {
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.slowThreshold)
		Warnf("%s | %s", slowLog, fmt.Sprintf(msg, float64(elapsed.Nanoseconds())/1e6, rows, sql))
		return
	}

	if l.logLevel >= gormlogger.Info {
		Infof(msg, float64(elapsed.Nanoseconds())/1e6, rows, sql)
	}
}
