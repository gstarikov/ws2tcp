package start

import (
	"context"
	"fmt"
	"github.com/kelseyhightower/envconfig"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
)

func PrepareEnv(cfg any) (context.Context, context.CancelFunc, *zap.Logger) {
	if err := envconfig.Process("", cfg); err != nil || len(os.Args) > 1 {
		fmt.Printf("bag config: %s\n", err)
		_ = envconfig.Usage("", cfg)
		os.Exit(1)
	}

	lcfg := zap.NewDevelopmentConfig()
	lcfg.DisableStacktrace = true
	lcfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	log, err := lcfg.Build()

	if err != nil {
		fmt.Printf("cant init logger: %s\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)

	log.Info("starting", zap.Any("config", cfg))

	return ctx, stop, log
}
