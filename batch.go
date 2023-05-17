package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

var batchCommand = &cli.Command{
	Name:   "batch",
	Usage:  "Batch command to generate a group of plots",
	Action: Batch,
	Flags: append([]cli.Flag{
		&cli.BoolFlag{
			Name:        "compact",
			Required:    false,
			Usage:       "Emit compact json instead of pretty-printed.",
			Destination: &batchOpts.compact,
		},
		&cli.BoolFlag{
			Name:        "validate",
			Required:    false,
			Usage:       "Validate the input files without running queries.",
			Destination: &batchOpts.validate,
		},
		&cli.StringSliceFlag{
			Name:        "source",
			Aliases:     []string{"s"},
			Required:    false,
			Usage:       "Specify the url of a data source, in the format name=url. May be repeated to specify multiple sources. Postgres urls take the form 'postgres://username:password@hostname:5432/database_name'",
			Destination: &batchOpts.sources,
		},
		&cli.StringFlag{
			Name:        "out",
			Required:    true,
			Usage:       "Path of directory where plots should be written.",
			Destination: &batchOpts.outDir,
		},
		&cli.StringFlag{
			Name:        "basis",
			Required:    false,
			Value:       "now",
			Usage:       "Basis time that should be passed to queries. Specify 'now' or a valid date in the past in RFC3339 or Unix timestamp format.",
			Destination: &batchOpts.basis,
		},
		&cli.BoolFlag{
			Name:        "version",
			Required:    true,
			Usage:       "Automatically version the plots by writing to a dated hierarchy.",
			Destination: &batchOpts.version,
		},
		&cli.BoolFlag{
			Name:        "force",
			Required:    false,
			Usage:       "Force generation of plots even if the plot output already exists.",
			Destination: &batchOpts.force,
		},
		&cli.IntFlag{
			Name:        "concurrency",
			Required:    false,
			Usage:       "Number of concurrent goroutines to use for generating plots.",
			Destination: &batchOpts.concurrency,
			Value:       6,
		},
		&cli.StringFlag{
			Name:        "conf",
			Required:    false,
			Usage:       "Path of directory containing configuration.",
			Destination: &batchOpts.confDir,
		},
	}, loggingFlags...),
}

var batchOpts struct {
	preview     bool
	compact     bool
	sources     cli.StringSlice
	outDir      string
	confDir     string
	validate    bool
	version     bool
	force       bool
	basis       string
	concurrency int
}

func Batch(cc *cli.Context) error {
	ctx := cc.Context
	setupLogging()

	if batchOpts.validate {
		// avoid interlacing output
		batchOpts.concurrency = 1
	}

	cfg := &PlotConfig{
		Sources: map[string]DataSource{
			"static": &StaticDataSource{},
			"demo":   &DemoDataSource{},
		},
		Colors: map[string]string{},
	}

	if batchOpts.basis == "now" {
		cfg.BasisTime = time.Now()
	} else {
		var err error
		ts, err := strconv.Atoi(batchOpts.basis)
		if err != nil {
			cfg.BasisTime, err = time.Parse(time.RFC3339, batchOpts.basis)
			if err != nil {
				return fmt.Errorf("invalid basis time: %w", err)
			}
		} else {
			cfg.BasisTime = time.Unix(int64(ts), 0)
		}

		if cfg.BasisTime.After(time.Now()) {
			return fmt.Errorf("basis time should not be in the future: %s", cfg.BasisTime.Format(time.RFC3339))
		}
	}
	cfg.BasisTime = cfg.BasisTime.UTC()
	slog.Info("plots will be generated for time " + cfg.BasisTime.Format(time.RFC3339))

	for _, sopt := range batchOpts.sources.Value() {
		name, url, ok := strings.Cut(sopt, "=")
		if !ok {
			return fmt.Errorf("source option not valid, use format 'name=url'")
		}

		if _, exists := cfg.Sources[name]; exists {
			return fmt.Errorf("duplicate source %q specified", name)
		}

		if strings.HasPrefix(url, "postgres:") {
			cfg.Sources[name] = NewPgDataSource(url)
		} else {
			return fmt.Errorf("unsupported source url: %q", url)
		}
	}

	if batchOpts.confDir != "" {
		conffs := os.DirFS(batchOpts.confDir)
		colorConfContent, err := fs.ReadFile(conffs, "colors.yaml")
		if err != nil {
			return fmt.Errorf("failed to read colors: %w", err)
		}

		var cd ColorDoc
		if err := yaml.Unmarshal(colorConfContent, &cd); err != nil {
			return fmt.Errorf("failed to unmarshal colors.yaml: %w", err)
		}

		cfg.DefaultColor = cd.Default
		cfg.Colors = make(map[string]string, len(cd.Colors))
		for _, nc := range cd.Colors {
			cfg.Colors[nc.Name] = nc.Color
		}

		profilesConfContent, err := fs.ReadFile(conffs, "profiles.yaml")
		if err != nil {
			return fmt.Errorf("failed to read profiles: %w", err)
		}

		var profiles []*ProcessingProfile
		if err := yaml.Unmarshal(profilesConfContent, &profiles); err != nil {
			return fmt.Errorf("failed to unmarshal processing profiles: %w", err)
		}

		for _, profile := range profiles {
			profile.Source = filepath.Join(batchOpts.confDir, profile.Source)

			if len(profile.Variants) == 0 {
				profile.Variants = []map[string]any{{}}
			}
		}
		cfg.Profiles = profiles
	}

	for _, profile := range cfg.Profiles {
		if err := profile.processPlotDefs(ctx, cfg); err != nil {
			return fmt.Errorf("processing plot definitions: %w", err)
		}
	}

	return nil
}

func (p *ProcessingProfile) processPlotDefs(ctx context.Context, cfg *PlotConfig) error {
	var (
		infs   fs.FS
		fnames []string
		err    error
	)
	if p.SourceIsDir() {
		infs = os.DirFS(p.Source)
		fnames, err = fs.Glob(infs, "*.yaml")
		if err != nil {
			return fmt.Errorf("failed to read input directory: %w", err)
		}
	} else {
		infs = os.DirFS(filepath.Dir(p.Source))
		fnames = []string{filepath.Base(p.Source)}
	}

	for _, variant := range p.Variants {

		// TODO: merge with existing TemplateParams as soon as the CLI option
		// was added.
		cfg.TemplateParams = variant

		grp, ctx := errgroup.WithContext(ctx)
		grp.SetLimit(batchOpts.concurrency)

		for _, fname := range fnames {
			fname := fname
			grp.Go(func() error {
				absOutDir, err := filepath.Abs(batchOpts.outDir)
				if err != nil {
					return fmt.Errorf("failed to find output directory: %w", err)
				}

				org := Organizer{
					Base:     absOutDir,
					Template: p.OutTpl,
					Params:   variant,
				}

				fcontent, err := fs.ReadFile(infs, fname)
				if err != nil {
					return fmt.Errorf("failed to read plot definition %q: %w", fname, err)
				}

				templated, err := ExecuteTemplate(ctx, string(fcontent), cfg)
				if err != nil {
					return fmt.Errorf("failed to execute templates for plot definition: %w", err)
				}

				pd, err := parsePlotDef(fname, []byte(templated))
				if err != nil {
					return fmt.Errorf("failed to parse plot definition %q: %w", fname, err)
				}

				logger := slog.With("name", pd.Name)
				plotFilename, err := org.Filepath(pd, cfg.BasisTime)
				if err != nil {
					return fmt.Errorf("plot filepath: %w", err)
				}
				logger.Debug("plot filename", "filepath", plotFilename)

				info, err := os.Lstat(filepath.Join(filepath.Dir(p.Source), fname))
				if err != nil {
					return err
				}

				isMissingOrStale, err := org.IsStaleOrMissing(pd, cfg.BasisTime, info.ModTime())
				if err != nil {
					logger.Error("failed to determine if plot file needs writing", "error", err)
				}

				shouldWrite := batchOpts.force || isMissingOrStale
				if shouldWrite {
					logger.Debug("plot file should be written")
				} else {
					logger.Debug("plot file does not need to be written")
				}

				isLatest, err := org.IsLatest(pd, cfg.BasisTime)
				if err != nil {
					logger.Error("failed to determine if plot file is latest", "error", err)
				}
				if isLatest {
					logger.Debug("plot is latest")
				} else {
					logger.Debug("plot is not latest")
				}

				if batchOpts.validate {
					fmt.Println("Name: " + pd.Name)
					fmt.Println("Frequency: " + pd.Frequency)
					fmt.Println("Output: " + plotFilename)
					fmt.Printf("Is missing or stale: %v\n", isMissingOrStale)
					fmt.Printf("Is latest version: %v\n", isLatest)

					fmt.Println("Datasets:")
					for _, ds := range pd.Datasets {
						fmt.Println("  Name: " + ds.Name)
						fmt.Println("  Source: " + ds.Source)
						fmt.Println("  Query:")
						fmt.Println(indent(ds.Query, "      "))

					}

					return nil
				}

				if !shouldWrite {
					slog.Info("skipping plot, output already exists", "name", pd.Name)
					return nil
				}

				slog.Info("generating plot", "name", pd.Name)
				// set up a monitoring loop that reports progress for long running queries
				done := make(chan struct{})
				t := time.NewTicker(time.Minute)
				go func() {
					start := time.Now()
					defer t.Stop()
					for {
						select {
						case <-t.C:
							slog.Info("still generating plot", "name", pd.Name, "elapsed", time.Since(start).Round(time.Second))
						case <-done:
							return
						}
					}
				}()
				fig, err := generateFig(ctx, pd, cfg)
				close(done) // stop the monitoring loop

				if err != nil {
					return fmt.Errorf("failed to generate plot %q: %w", pd.Name, err)
				}

				figDat := FigureData{
					Fig:    fig,
					Params: pd.Parameters,
				}

				var data []byte
				if batchOpts.compact {
					data, err = json.Marshal(figDat)
				} else {
					data, err = json.MarshalIndent(figDat, "", "  ")
				}
				if err != nil {
					return fmt.Errorf("failed to marshal to json: %w", err)
				}

				slog.Info("writing plot output", "name", pd.Name, "filename", plotFilename)
				if err := org.WritePlot(data, pd, cfg.BasisTime); err != nil {
					return fmt.Errorf("failed to write plot: %w", err)
				}

				return nil
			})
		}

		if err := grp.Wait(); err != nil {
			return err
		}
	}

	return nil
}

func fileExists(fname string) (bool, error) {
	_, err := os.Lstat(fname)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return true, fmt.Errorf("failed to stat file: %w", err)
	}

	return false, nil
}

func fileExistsAndIsNewerThan(fname string, ts time.Time) (bool, error) {
	info, err := os.Lstat(fname)
	if err == nil {
		return info.ModTime().After(ts), nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return true, fmt.Errorf("failed to stat file: %w", err)
	}

	return false, nil
}

func writeOutput(fname string, data []byte) error {
	dir := filepath.Dir(fname)
	if err := os.MkdirAll(dir, 0o775); err != nil {
		return fmt.Errorf("make directories: %w", err)
	}

	f, err := os.Create(fname)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, string(data))
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}
