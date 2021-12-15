package paths

import (
	"fmt"
	"os"
	"path"
	"strconv"
)

const (
	ResultsDir = "./tools/experiment/results"
	LogsPath   = "./tools/experiment/logs"
)

var (
	FinalPath   = ""
	LogFilename = ""
)

func CreateResultsDir(k int, custom string) {
	kPath := path.Join(ResultsDir, fmt.Sprintf("k_%d", k))
	customPath := path.Join(kPath, custom)
	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		FinalPath = path.Join(customPath, "0")
		err = os.MkdirAll(FinalPath, 0700)
		if err != nil {
			panic(err)
		}
	} else {
		// path already existed create next sub folder with number as a name
		entry, _ := os.ReadDir(customPath)
		lastDirName := len(entry)
		FinalPath = path.Join(customPath, strconv.Itoa(lastDirName))

		err = os.Mkdir(FinalPath, 0700)
		if err != nil {
			panic(err)
		}
	}
}

func MoveLogFile() {
	err := os.Rename(path.Join(LogsPath, LogFilename), path.Join(FinalPath, LogFilename))
	if err != nil {
		panic(err)
	}
}
