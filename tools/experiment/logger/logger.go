package logger

import (
	"github.com/iotaledger/hive.go/configuration"
	"github.com/iotaledger/hive.go/logger"
)

func init() {
	if err := logger.InitGlobalLogger(configuration.New()); err != nil {
		panic(err)
	}
	logger.SetLevel(logger.LevelInfo)
}

var New = logger.NewLogger
