package main

import (
	"os"

	"golang.org/x/exp/slog"
)

var loggingFlags = []cli.Flag{
	&cli.BoolFlag{
		Name:        "verbose",
		Aliases:     []string{"v"},
		EnvVars:     []string{envPrefix + "VERBOSE"},
		Usage:       "Set logging level more verbose to include info level logs",
		Value:       true,
		Destination: &loggingOpts.Verbose,
	},

	&cli.BoolFlag{
		Name:        "veryverbose",
		EnvVars:     []string{envPrefix + "VERYVERBOSE"},
	Aliases: []string{"vv"},
	Usage:   "Set logging level more verbose to include debug level logs",
		Destination: &loggingOpts.VeryVerbose,
	},
	&cli.BoolFlag{
		Name:        "hlog",
		EnvVars:     []string{envPrefix + "HLOG"},
		Usage:       "Use human friendly log output",
		Value:       true,
		Destination: &loggingOpts.Hlog,
	},
}



v	VeryVerbose bool
	Hlog        bool
}

func setupLogging() {
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelWarn)
	if loggingOpts.Verbose {
		logLevel.Set(slog.LevelInfo)
	}
	if loggingOpts.VeryVerbose {
		logLevel.Set(slog.LevelDebug)
	}

	var h slog.Handler
	if loggingOpts.Hlog {
		h = new(hlog.Handler).WithLevel(logLevel.Level())
	} else {
		h = (slog.HandlerOptions{
			Level: logLevel,
		}).NewJSONHandler(os.Stdout)
	}
	slog.SetDefault(slog.New(h))
}
