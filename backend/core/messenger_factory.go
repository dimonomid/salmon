package core

import (
	"fmt"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/itemsboard"
	"github.com/dimonomid/salmon/backend/messengers"
	"github.com/dimonomid/salmon/backend/messengers/filelogger"
	"github.com/dimonomid/salmon/backend/messengers/webserver"
)

// messengerWCtx contains the messenger and its context (e.g. channels for that
// messenger)
type messengerWCtx struct {
	messenger messengers.Messenger

	notificationsChan chan *salmon.Notification
	tornDown          chan struct{}
}

func createMessenger(
	cfg Messenger, commonParams messengers.Params,
) (messengers.Messenger, error) {
	var ret messengers.Messenger

	if cfg.FileLogger != nil {
		if ret != nil {
			return nil, fmt.Errorf("config contains more than a single messenger")
		}

		c, err := filelogger.New(filelogger.Params{
			Common: commonParams,
			Config: *cfg.FileLogger,
		})
		if err != nil {
			return nil, fmt.Errorf("creating filelogger: %w", err)
		}

		return c, nil
	}

	if cfg.Webserver != nil {
		if ret != nil {
			return nil, fmt.Errorf("config contains more than a single messenger")
		}

		c, err := webserver.New(webserver.Params{
			Common: commonParams,
			Config: *cfg.Webserver,
		})
		if err != nil {
			return nil, fmt.Errorf("creating webserver: %w", err)
		}

		return c, nil
	}

	return nil, fmt.Errorf("no valid messenger configuration")
}

func createMessengers(cfgs []Messenger, ib *itemsboard.ItemsBoard) ([]messengerWCtx, error) {
	ret := make([]messengerWCtx, 0, len(cfgs))

	for i, cfg := range cfgs {
		mwCtx := messengerWCtx{
			notificationsChan: make(chan *salmon.Notification, 32),
			tornDown:          make(chan struct{}),
		}

		commonParams := messengers.Params{
			ItemsBoard:        ib,
			NotificationsChan: mwCtx.notificationsChan,
			TornDown:          mwCtx.tornDown,
		}

		var err error
		mwCtx.messenger, err = createMessenger(cfg, commonParams)
		if err != nil {
			return nil, fmt.Errorf("creating messenger from config #%d: %w", i, err)
		}

		ret = append(ret, mwCtx)
	}

	return ret, nil
}
