package logger

import (
	"fmt"
	"github.com/iotaledger/goshimmer/tools/experiment/paths"
	"github.com/iotaledger/hive.go/configuration"
	"github.com/iotaledger/hive.go/logger"
	"os"
	"time"
)

func init() {
	config := configuration.New()
	// save current pwd
	currDir, err := os.Getwd()
	fmt.Println(currDir, paths.FinalPath)
	if err != nil {
		panic(err)
	}
	// create directory for logs if not exists
	if _, err := os.Stat(paths.LogsPath); os.IsNotExist(err) {
		err = os.Mkdir(paths.LogsPath, 0700)
		if err != nil {
			panic(err)
		}
	}
	err = os.Chdir(paths.LogsPath)
	if err != nil {
		panic(err)
	}
	paths.LogFilename = fmt.Sprintf("orphanage-tests-%s.log", time.Now().Format("020106_030405PM"))
	err = config.Set(logger.ConfigurationKeyOutputPaths, []string{"stdout", paths.LogFilename})
	if err != nil {
		fmt.Println(err)
		return
	}

	if err := logger.InitGlobalLogger(config); err != nil {
		panic(err)
	}
	logger.SetLevel(logger.LevelInfo)
	// set back the previous pwd
	err = os.Chdir(currDir)
	if err != nil {
		panic(err)
	}
}

var New = logger.NewLogger
