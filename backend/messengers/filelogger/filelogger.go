package filelogger

import (
	"fmt"
	"os"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/messengers"
)

const timeFmt = "2006-01-02 15:04:05.000"

type FileLogger struct {
	params Params

	f *os.File
}

var _ messengers.Messenger = &FileLogger{}

type Params struct {
	Common messengers.Params

	Config Config
}

func New(params Params) (*FileLogger, error) {
	var f *os.File

	if params.Config.FileName != "" {
		var err error
		f, err = os.OpenFile(params.Config.FileName, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("opening log file: %w", err)
		}
	} else {
		f = os.Stdout
	}

	fl := &FileLogger{
		params: params,

		f: f,
	}

	go fl.run()

	return fl, nil
}

func (fl *FileLogger) String() string {
	filename := fl.params.Config.FileName
	if filename == "" {
		filename = "stdout"
	}

	return fmt.Sprintf("filelogger to %s", filename)
}

func (fl *FileLogger) run() {
	for notif := range fl.params.Common.NotificationsChan {
		nt := notif.Time.Format(timeFmt)

		for _, item := range notif.OngoingIncidents.Removed {
			fmt.Fprintf(fl.f, "%s [ %s ] %s\n", nt, salmon.ItemStateOK, item.Key)
		}

		for _, item := range notif.OngoingIncidents.Added {
			fmt.Fprintf(fl.f, "%s [ %s ] %s (%s)\n", nt, item.State, item.Key, item.Comment)
		}

		for _, item := range notif.OngoingIncidents.Updated {
			fmt.Fprintf(fl.f, "%s [ %s ][ updated ] %s (%s)\n", nt, item.State, item.Key, item.Comment)
		}

		// TODO: handle one-off incidents as well
	}

	// Input channel was closed, so teardown now: unless we're writing to stdout,
	// close the output file.

	if fl.params.Config.FileName != "" {
		if err := fl.f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close output file: %s\n", err.Error())
		}
	}

	close(fl.params.Common.TornDown)
}
