package main

import (
	"os"
	"time"

	"github.com/labstack/gommon/log"

	"github.com/hesusruiz/signers/history"
	"github.com/hesusruiz/signers/logfilter"
	"github.com/hesusruiz/signers/redt"
	"github.com/hesusruiz/signers/serve"
	"github.com/urfave/cli/v2"
)

// The default node address used is a local one
var localNodeHTTP = "http://127.0.0.1:22000"
var localNodeWS = "ws://127.0.0.1:22001"

func main() {

	// Define commands, parse command line arguments and start execution
	app := &cli.App{
		Usage: "Monitoring of block signers activity for the Alastria RedT blockchain network",
		UsageText: `signers [global options] command [command options]
			where 'nodeURL' is the address of the Quorum node.
			It supports both HTTP and WebSockets endpoints.
			By default it uses WebSockets and for HTTP you have to use the 'poll' subcommand.`,

		EnableBashCompletion:   true,
		UseShortOptionHandling: true,
		Version:                "v0.2",
		Compiled:               time.Now(),
		Authors: []*cli.Author{
			{
				Name:  "Jesus Ruiz",
				Email: "hesus.ruiz@gmail.com",
			},
		},

		Action: func(c *cli.Context) error {
			cli.ShowAppHelpAndExit(c, 0)
			return nil
		},
	}

	monitorWSCMD := &cli.Command{
		Name:      "monitor",
		Usage:     "monitor the signers activity via WebSockets events",
		UsageText: "signers monitor [options]",

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "url",
				Value:    localNodeWS,
				Usage:    "ws url of the endpoint of blockchain node",
				Aliases:  []string{"u"},
				Required: false,
			},
			&cli.Int64Flag{
				Name:    "blocks",
				Value:   10,
				Usage:   "number of blocks in the past to process",
				Aliases: []string{"b"},
			},
		},

		Action: func(c *cli.Context) error {
			url := c.String("url")
			numBlocks := c.Int64("blocks")
			redt.MonitorSignersWS(url, numBlocks)
			return nil
		},
	}

	monitorCMD := &cli.Command{
		Name:      "poll",
		Usage:     "monitor the signers activity via HTTP polling",
		UsageText: "signers poll [options]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "url",
				Value:    localNodeHTTP,
				Usage:    "url of the endpoint of blockchain node",
				Aliases:  []string{"u"},
				Required: false,
			},
			&cli.Int64Flag{
				Name:    "blocks",
				Value:   10,
				Usage:   "number of blocks in the past to process before starting",
				Aliases: []string{"b"},
			},
			&cli.Int64Flag{
				Name:    "refresh",
				Value:   2,
				Usage:   "refresh interval for presentation. All blocks are processed independent of this value",
				Aliases: []string{"r"},
			},
		},

		Action: func(c *cli.Context) error {
			url := c.String("url")
			numBlocks := c.Int64("blocks")
			refresh := c.Int64("refresh")
			redt.MonitorSigners(url, numBlocks, refresh)
			return nil
		},
	}

	displayPeersCMD := &cli.Command{
		Name:  "peers",
		Usage: "display peers information",

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "url",
				Value:    localNodeHTTP,
				Usage:    "url of the endpoint of blockchain node",
				Aliases:  []string{"u"},
				Required: false,
			},
		},

		Action: func(c *cli.Context) error {
			url := c.String("url")
			redt.DisplayPeersInfo(url)
			return nil
		},
	}

	logfilterCMD := &cli.Command{
		Name:  "logfilter",
		Usage: "display filtered log information",

		Action: func(c *cli.Context) error {
			logfilter.FilterLogs()
			return nil
		},
	}

	serveCMD := &cli.Command{
		Name:      "serve",
		Usage:     "run a web server to display signers behaviour in real time",
		UsageText: "signers serve [options]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "url",
				Value:    localNodeWS,
				Usage:    "url of the endpoint of blockchain node",
				Aliases:  []string{"u"},
				Required: false,
			},
			&cli.StringFlag{
				Name:    "ip",
				Value:   "0.0.0.0",
				Usage:   "IP address of the web server",
				Aliases: []string{"i"},
			},
			&cli.Int64Flag{
				Name:    "port",
				Value:   8080,
				Usage:   "port of the IP address for the web server",
				Aliases: []string{"p"},
			},
		},

		Action: func(c *cli.Context) error {
			url := c.String("url")
			ip := c.String("ip")
			port := c.Int64("port")
			serve.ServeSigners(url, ip, port)
			return nil
		},
	}

	historyCMD := &cli.Command{
		Name:      "history",
		Usage:     "download blockchain headers into SQLite database, from current towards genesis",
		UsageText: "signers poll [options]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "url",
				Value:    localNodeHTTP,
				Usage:    "url of the endpoint of blockchain node",
				Aliases:  []string{"u"},
				Required: false,
			},
			&cli.StringFlag{
				Name:     "dsn",
				Value:    "./blockchain.sqlite?_journal=WAL",
				Usage:    "dsn of the SQLite database",
				Aliases:  []string{"d"},
				Required: false,
			},
			&cli.BoolFlag{
				Name:     "stats",
				Value:    false,
				Usage:    "dsn of the SQLite database",
				Aliases:  []string{"s"},
				Required: false,
			},
		},

		Action: func(c *cli.Context) error {
			url := c.String("url")
			dsn := c.String("dsn")
			stats := c.Bool("stats")
			err := history.HistoryBackwards(url, dsn, stats)
			if err != nil {
				log.Error(err)
			}
			return err
		},
	}

	historyForwardCMD := &cli.Command{
		Name:      "historyfw",
		Usage:     "download blockchain headers into SQLite database, from newest stored towards current",
		UsageText: "signers poll [options]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "url",
				Value:    localNodeHTTP,
				Usage:    "url of the endpoint of blockchain node",
				Aliases:  []string{"u"},
				Required: false,
			},
			&cli.StringFlag{
				Name:     "dsn",
				Value:    "./blockchain.sqlite?_journal=WAL",
				Usage:    "dsn of the SQLite database",
				Aliases:  []string{"d"},
				Required: false,
			},
			&cli.BoolFlag{
				Name:     "stats",
				Value:    false,
				Usage:    "dsn of the SQLite database",
				Aliases:  []string{"s"},
				Required: false,
			},
		},

		Action: func(c *cli.Context) error {
			url := c.String("url")
			dsn := c.String("dsn")
			stats := c.Bool("stats")
			err := history.HistoryForward(url, dsn, stats)
			if err != nil {
				log.Error(err)
			}
			return err
		},
	}

	app.Commands = []*cli.Command{
		monitorWSCMD,
		monitorCMD,
		displayPeersCMD,
		logfilterCMD,
		serveCMD,
		historyCMD,
		historyForwardCMD,
	}

	// Run the application
	err := app.Run(os.Args)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

}
