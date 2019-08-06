// Copyright 2019 github.com/ucirello
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"cirello.io/runner/runner"
	"github.com/gorilla/websocket"
	cli "github.com/urfave/cli"
)

func logs() cli.Command {
	return cli.Command{
		Name: "logs",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "filter",
				Usage: "service name to filter message",
			},
		},
		Action: func(c *cli.Context) error {
			interrupt := make(chan os.Signal, 1)
			signal.Notify(interrupt, os.Interrupt)

			u := url.URL{Scheme: "ws", Host: c.GlobalString("service-discovery"), Path: "/logs"}
			if filter := c.String("filter"); filter != "" {
				query := u.Query()
				query.Set("filter", filter)
				u.RawQuery = query.Encode()
			}
			log.Printf("connecting to %s", u.String())

			follow := func() error {
				ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
				if err != nil {
					return fmt.Errorf("cannot dial to service discovery endpoint: %v s", err)
				}
				defer ws.Close()

				done := make(chan struct{})
				go func() {
					defer close(done)
					for {
						_, message, err := ws.ReadMessage()
						if err != nil {
							log.Println("read:", err)
							return
						}
						var msg runner.LogMessage
						if err := json.Unmarshal(message, &msg); err != nil {
							log.Println("decode:", err)
							continue
						}
						fmt.Println(msg.PaddedName+":", msg.Line)
					}
				}()
				for {
					select {
					case <-done:
						return nil
					case <-interrupt:
						log.Println("interrupt")
						err := ws.WriteMessage(
							websocket.CloseMessage,
							websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
						)
						if err != nil {
							log.Println("write close:", err)
							return nil
						}
						select {
						case <-done:
						case <-time.After(time.Second):
						}
						return nil
					}
				}
			}
			var err error
			for {
				select {
				case <-interrupt:
					return err
				default:
					err = follow()
				}
			}
		},
	}
}
