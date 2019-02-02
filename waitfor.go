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
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	cli "gopkg.in/urfave/cli.v1"
)

func waitfor() {
	app := cli.NewApp()
	app.Name = "waitfor"
	app.Usage = "probes the service discovery before running the given command"
	app.HideVersion = true
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "service-discovery",
			Value: "localhost:64000",
			Usage: "service discovery address",
		},
	}
	app.Action = func(c *cli.Context) error {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-sigint
			log.Println("shutting down")
			cancel()
		}()

		for {
			time.Sleep(250 * time.Millisecond)
			resp, err := http.Get("http://" + c.String("service-discovery") + "/discovery")
			if err != nil {
				continue
			}
			defer resp.Body.Close()
			var procs map[string]string // map of procType to state
			if err := json.NewDecoder(resp.Body).Decode(&procs); err != nil {
				return fmt.Errorf("cannot parse state of service discovery endpoint: %v", err)
			}
			allBuilt := true
			for k, v := range procs {
				if !strings.HasPrefix(k, "BUILD_") || v == "done" {
					continue
				}
				allBuilt = false
				break

			}
			if allBuilt {
				break
			}
		}

		cmd := strings.Join(c.Args(), " ")
		cmdExec := exec.CommandContext(ctx, "sh", "-c", cmd)
		cmdExec.Stdin = os.Stdin
		cmdExec.Stderr = os.Stderr
		cmdExec.Stdout = os.Stdout
		cmdExec.Run()

		return nil
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
