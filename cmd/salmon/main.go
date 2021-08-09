package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dimonomid/salmon"
	"github.com/dimonomid/salmon/backend/collectors/systemd"
	"github.com/dimonomid/salmon/backend/core"
	"github.com/dimonomid/salmon/backend/messengers/filelogger"

	"github.com/benbjohnson/clock"
)

func main() {
	c, err := core.NewCore(
		core.Config{
			Collectors: []core.Collector{
				core.Collector{
					ID: "mysystemd",
					Systemd: &systemd.Config{
						UnitRules: []systemd.ConfigUnitRule{
							systemd.ConfigUnitRule{
								Name: "my-iptables.service",
								Conds: []systemd.ConfigCond{
									systemd.ConfigCond{State: "active", Result: salmon.ItemStateOK},
									systemd.ConfigCond{Result: salmon.ItemStateError},
								},
							},

							// TODO: make sure we get errors due to that non-existing service
							systemd.ConfigUnitRule{
								Name: "some-non-existing.service",
								Conds: []systemd.ConfigCond{
									systemd.ConfigCond{State: "active", Result: salmon.ItemStateOK},
									systemd.ConfigCond{Result: salmon.ItemStateError},
								},
							},
							systemd.ConfigUnitRule{
								Type: "service",
								Conds: []systemd.ConfigCond{
									systemd.ConfigCond{State: "active", Result: salmon.ItemStateOK},
									systemd.ConfigCond{State: "inactive", Result: salmon.ItemStateOK},
									systemd.ConfigCond{State: "activating", Result: salmon.ItemStateOK},
									systemd.ConfigCond{State: "deactivating", Result: salmon.ItemStateOK},
									systemd.ConfigCond{State: systemd.UnitStateNotSentBySystemd, Result: salmon.ItemStateOK},
									systemd.ConfigCond{Result: salmon.ItemStateError},
								},
							},
						},
					},
				},
			},
			Messengers: []core.Messenger{
				core.Messenger{
					FileLogger: &filelogger.Config{
						FileName: "/var/tmp/my_salmon_log",
					},
				},
				core.Messenger{
					FileLogger: &filelogger.Config{
						FileName: "", // stdout
					},
				},
			},
		},
		core.Params{
			Clock: clock.New(),
		},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize salmon core: %s\n", err)
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	<-sigCh
	fmt.Println("Exiting...")
	c.Close()

	fmt.Println("Bye.")
}
