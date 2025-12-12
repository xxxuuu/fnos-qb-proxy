package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path"

	"github.com/urfave/cli/v2"
)

func proxyCmd(ctx *cli.Context) error {
	uds := ctx.String("uds")
	debug := ctx.Bool("debug")
	port := ctx.Int("port")
	expectedPassword := ctx.String("password")
	proxy := NewFnosProxy(uds, debug, expectedPassword, port)
	fmt.Printf("proxy running on port %d\n", port)
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), proxy)
	if err != nil {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

func main() {
	home, _ := os.UserHomeDir()
	app := &cli.App{
		Name:   "fnos-qb-proxy",
		Usage:  "fnos-qb-proxy is a proxy for qBittorrent in fnOS",
		Action: proxyCmd,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "password",
				Aliases: []string{"p"},
				Usage:   "if not set, qBittorrent will login automatically",
				Value:   "",
			},
			&cli.StringFlag{
				Name:  "uds",
				Usage: "qBittorrent unix domain socket(uds) path",
				Value: path.Join(home, "qbt.sock"),
			},
			&cli.BoolFlag{
				Name:    "debug",
				Aliases: []string{"d"},
				Value:   false,
			},
			&cli.IntFlag{
				Name:  "port",
				Usage: "proxy running port",
				Value: 8080,
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
