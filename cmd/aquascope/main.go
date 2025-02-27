//go:generate go-bindata-assetfs -pkg main -nocompress -modtime 1 -mode 420 assets/...
//go:generate goimports -w -format-only -local=code.cryptowat.ch ./bindata.go

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"strings"

	"github.com/0xAX/notificator"
	"github.com/benbjohnson/clock"
	assetfs "github.com/elazarl/go-bindata-assetfs"
	"github.com/getlantern/systray"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/pflag"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/wsclient"
)

const (
	defPort = 41991
)

var notify *notificator.Notificator

var (
	iconSalmonGray    []byte
	iconSalmonGreen   []byte
	iconSalmonMagenta []byte
	iconSalmonYellow  []byte
	iconSalmonRed     []byte
)

func main() {
	// NOTE: Run should be the only thing which executes in main; otherwise
	// the magic done by runtime.LockOSThread() (which is called from
	// systray.Run) might not work.
	systray.Run(onReady, onExit)
}

func onReady() {
	usr, err := user.Current()
	if err != nil {
		panic(err.Error())
	}
	fmt.Println(usr.HomeDir)

	configFilename := pflag.String(
		"config", fmt.Sprintf("%s/.config/aquascope/aquascope.yml", usr.HomeDir), "Config filename",
	)

	pflag.Parse()

	cfg, err := loadConfig(*configFilename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read config from %s: %s\n", configFilename, err)
		os.Exit(1)
	}

	mitemStatus := systray.AddMenuItem("Status", "")
	mitemExit := systray.AddMenuItem("Exit", "")

	notify = notificator.New(notificator.Options{})
	notify.Push("Hello there", "Aquascope started", "", notificator.UR_NORMAL)

	iconSalmonGray = MustAsset("assets/salmon_gray.png")
	iconSalmonGreen = MustAsset("assets/salmon_green.png")
	iconSalmonMagenta = MustAsset("assets/salmon_magenta.png")
	iconSalmonYellow = MustAsset("assets/salmon_yellow.png")
	iconSalmonRed = MustAsset("assets/salmon_red.png")

	applyIcon(overallStateUnknown)

	c, err := wsclient.NewCombiner(wsclient.CombinerParams{
		Config: cfg.WSClient,

		OngoingIncidentsHandler: func(notif *salmon.Notification) {
			d, _ := json.MarshalIndent(notif, "", "  ")
			fmt.Println(string(d))

			for _, item := range notif.OngoingIncidents.Added {
				notify.Push(string(item.State)+": "+string(item.Key), item.Comment, "", notificator.UR_NORMAL)
			}

			for _, item := range notif.OngoingIncidents.Updated {
				notify.Push("updated "+string(item.State)+": "+string(item.Key), item.Comment, "", notificator.UR_NORMAL)
			}

			for _, item := range notif.OngoingIncidents.Removed {
				notify.Push("OK: "+string(item.Key), "", "", notificator.UR_NORMAL)
			}

			state := getOverallStateFromItems(notif.OngoingIncidents.Total)
			fmt.Println("Overall state:", state)

			applyIcon(state)
		},

		Clock: clock.New(),
	})
	if err != nil {
		fmt.Println("Error creating client:", err)
		os.Exit(1)
	}

	_ = c

	listener := setupWebserver()
	port := listener.Addr().(*net.TCPAddr).Port

	fmt.Printf("Listening on %d\n", port)

	go func() {
		panic(http.Serve(listener, nil))
	}()

	go func() {
		for {
			select {
			case <-mitemStatus.ClickedCh:
				open.Run(fmt.Sprintf("http://localhost:%d/status", port))

			case <-mitemExit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

// setupWebserver tries to create a listener on the default port (defPort);
// if that fails for whatever reason, tries to listen on a random port, and if
// that fails as well, panics.
func setupWebserver() net.Listener {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", defPort))
	if err != nil {
		fmt.Printf("Failed to listen on a default port %d (%s), listening on random port\n", defPort, err)
		listener, err = net.Listen("tcp", ":0")
		if err != nil {
			panic(err.Error())
		}
	}

	http.HandleFunc("/status", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("TODO status"))
	})

	http.Handle("/", webrootFileServer())

	return listener
}

func webrootFileServer() http.Handler {
	assetInfo := func(path string) (os.FileInfo, error) {
		return os.Stat(path)
	}

	return http.FileServer(
		&assetfs.AssetFS{
			Asset:     Asset,
			AssetDir:  AssetDir,
			AssetInfo: assetInfo,
			Prefix:    "assets/webroot",
		},
	)
}

func onExit() {
	fmt.Println("Exiting")
}

type overallState int

const (
	overallStateUnknown overallState = iota
	overallStateOK
	overallStateInternalError
	overallStateWarning
	overallStateError
)

func getOverallStateFromItems(items []*salmon.ItemWContext) overallState {
	ret := overallStateOK

	for _, item := range items {
		cur := getOverallStateFromItem(item)
		if ret < cur {
			ret = cur
		}
	}

	return ret
}

func getOverallStateFromItem(item *salmon.ItemWContext) overallState {
	if item.State == salmon.ItemStateOK {
		return overallStateOK
	}

	if strings.Contains(string(item.Key), "internal") {
		return overallStateInternalError
	}

	if item.State == salmon.ItemStateWarning {
		return overallStateWarning
	}

	return overallStateError
}

func sendNotification(title, text string) {
	notify.Push(title, text, "", notificator.UR_NORMAL)
}

func applyIcon(state overallState) {
	var icon []byte

	switch state {
	case overallStateUnknown:
		icon = iconSalmonGray
	case overallStateOK:
		icon = iconSalmonGreen
	case overallStateInternalError:
		icon = iconSalmonMagenta
	case overallStateWarning:
		icon = iconSalmonYellow
	case overallStateError:
		icon = iconSalmonRed
	default:
		panic(fmt.Sprintf("invalid state %d", state))
	}

	systray.SetIcon(icon)
}
