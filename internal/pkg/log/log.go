package log

import (
	"log/slog"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
)

func SetupLogger(serviceName string) {
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncodeLevel = zapcore.CapitalLevelEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(config),
		zapcore.AddSync(os.Stdout),
		zap.InfoLevel,
	)

	zapLogger := zap.New(core).With(zap.String("service", serviceName))
	handler := zapslog.NewHandler(zapLogger.Core())
	logger := slog.New(handler)
	slog.SetDefault(logger)
}
