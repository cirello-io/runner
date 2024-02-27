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
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"cirello.io/runner/runner"
	cli "github.com/urfave/cli"
	"nhooyr.io/websocket"
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
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			u := url.URL{Scheme: "ws", Host: c.GlobalString("service-discovery"), Path: "/logs"}
			if filter := c.String("filter"); filter != "" {
				query := u.Query()
				query.Set("filter", filter)
				u.RawQuery = query.Encode()
			}
			log.Printf("connecting to %s", u.String())

			follow := func() (outErr error) {
				ws, _, err := websocket.Dial(ctx, u.String(), nil)
				if err != nil {
					return fmt.Errorf("cannot dial to service discovery endpoint: %v s", err)
				}
				defer func() {
					err := ws.CloseNow()
					if outErr == nil && err != nil {
						outErr = err
					}
				}()

				done := make(chan struct{})
				go func() {
					defer close(done)
					for {
						_, message, err := ws.Read(ctx)
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
					case <-ctx.Done():
						log.Println("interrupt")
						ws.Close(websocket.StatusNormalClosure, "")
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
				case <-ctx.Done():
					return err
				default:
					err = follow()
				}
			}
		},
	}
}
