package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/logutils"
)

var (
	now = time.Now
)

//LogFile is used to setup a file based logger that also performs log rotation
type LogFile struct {
	// Log level Filter to filter out logs that do not matcch LogLevel criteria
	logFilter *logutils.LevelFilter

	//Name of the log file
	fileName string

	//full Name of the log file
	fullName string

	//rotate Name of the log file
	rotateName string

	//Path to the log file
	logPath string

	//Duration between each file rotation operation
	duration time.Duration

	//LastCreated represents the creation time of the latest log
	LastCreated time.Time

	//FileInfo is the pointer to the current file being written to
	FileInfo *os.File

	//MaxBytes is the maximum number of desired bytes for a log file
	MaxBytes int

	//BytesWritten is the number of bytes written in the current log file
	BytesWritten int64

	// Max rotated files to keep before removing them.
	MaxFiles int

	//acquire is the mutex utilized to ensure we have no concurrency issues
	acquire sync.Mutex
}

func (l *LogFile) fileNamePattern() string {
	// Extract the file extension
	fileExt := filepath.Ext(l.fileName)
	// If we have no file extension we append .log
	if fileExt == "" {
		fileExt = ".log"
	}
	// Remove the file extension from the filename
	return strings.TrimSuffix(l.fileName, fileExt) + "%s" + fileExt
}

func (l *LogFile) openNew() error {
	fileNamePattern := l.fileNamePattern()
	// New file name has the format : filename-timestamp.extension
	createTime := now()
	//newfileName := fmt.Sprintf(fileNamePattern, strconv.FormatInt(createTime.UnixNano(), 10))
	l.rotateName = fmt.Sprintf(fileNamePattern, "-"+time.Unix(createTime.Unix(), 0).Format("20060102150405"))
	newfileName := fmt.Sprintf(fileNamePattern, "")
	newfilePath := filepath.Join(l.logPath, newfileName)
	l.fullName = newfilePath
	l.rotateName = filepath.Join(l.logPath, l.rotateName)
	// Try creating a file. We truncate the file because we are the only authority to write the logs
	filePointer, err := os.OpenFile(newfilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	l.FileInfo = filePointer
	// New file, new bytes tracker, new creation time :)
	l.LastCreated = createTime
	l.BytesWritten = 0
	return nil
}

func (l *LogFile) rotate() error {
	// Get the time from the last point of contact
	timeElapsed := time.Since(l.LastCreated)
	// Rotate if we hit the byte file limit or the time limit
	if (l.BytesWritten >= int64(l.MaxBytes) && (l.MaxBytes > 0)) || timeElapsed >= l.duration {
		l.FileInfo.Close()
		os.Rename(l.fullName, l.rotateName)
		//if err := l.pruneFiles(); err != nil {
		//	return err
		//}

		//delete old files(>30 days)
		filepath.Walk(filepath.Dir(l.fullName), func(path string, f os.FileInfo, err error) error {
			if f == nil {
				return err
			}
			if f.IsDir() {
				return nil
			}
			if !strings.HasSuffix(f.Name(), ".log") {
				return nil
			}
			if time.Since(f.ModTime()) > 30*24*time.Hour {
				os.Remove(path)
			} else { //if file is not old enough ,skip this process
				return filepath.SkipDir
			}
			return nil
		})
		return l.openNew()
	}
	return nil
}

func (l *LogFile) pruneFiles() error {
	if l.MaxFiles == 0 {
		return nil
	}
	pattern := l.fileNamePattern()
	//get all the files that match the log file pattern
	globExpression := filepath.Join(l.logPath, fmt.Sprintf(pattern, "*"))
	matches, err := filepath.Glob(globExpression)
	if err != nil {
		return err
	}
	// Prune if there are more files stored than the configured max
	stale := len(matches) - l.MaxFiles
	for i := 0; i < stale; i++ {
		if err := os.Remove(matches[i]); err != nil {
			return err
		}
	}
	return nil
}

// Write is used to implement io.Writer
func (l *LogFile) Write(b []byte) (n int, err error) {
	// Filter out log entries that do not match log level criteria
	if !l.logFilter.Check(b) {
		return 0, nil
	}

	l.acquire.Lock()
	defer l.acquire.Unlock()
	//Create a new file if we have no file to write to
	if l.FileInfo == nil {
		if err := l.openNew(); err != nil {
			return 0, err
		}
	}
	// Check for the last contact and rotate if necessary
	if err := l.rotate(); err != nil {
		return 0, err
	}
	l.BytesWritten += int64(len(b))
	return l.FileInfo.Write(b)
}
