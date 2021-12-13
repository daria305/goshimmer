package logger

import (
	"fmt"
	"github.com/iotaledger/hive.go/configuration"
	"github.com/iotaledger/hive.go/logger"
	"os"
	"time"
)

const (
	path = "./tools/experiment/logs/"
)

func init() {
	config := configuration.New()
	// create directory for logs if not exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err = os.Mkdir(path, 0700)
		if err != nil {
			panic(err)
		}
	}
	err := os.Chdir(path)
	if err != nil {
		panic(err)
	}
	filename := fmt.Sprintf("orphanage-tests-%s.log", time.Now().Format("020106_030405PM"))

	err = config.Set(logger.ConfigurationKeyOutputPaths, []string{"stdout", filename})
	if err != nil {
		fmt.Println(err)
		return
	}
	if err := logger.InitGlobalLogger(config); err != nil {
		panic(err)
	}
	logger.SetLevel(logger.LevelInfo)
}

var New = logger.NewLogger
