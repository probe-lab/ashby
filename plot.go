package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/exp/slog"
	"gopkg.in/yaml.v3"
)

var plotCommand = &cli.Command{
	Name:   "plot",
	Usage:  "Interactive command to generate a single plot",
	Action: Plot,
	Flags: append([]cli.Flag{
		&cli.BoolFlag{
			Name:        "preview",
			Required:    false,
			Usage:       "Preview the plot in a browser window.",
			Destination: &plotOpts.preview,
		},
		&cli.BoolFlag{
			Name:        "compact",
			Required:    false,
			Usage:       "Emit compact json instead of pretty-printed.",
			Destination: &plotOpts.compact,
		},
		&cli.BoolFlag{
			Name:        "validate",
			Required:    false,
			Usage:       "Validate the input file without running queries.",
			Destination: &plotOpts.validate,
		},
		&cli.StringSliceFlag{
			Name:        "source",
			Aliases:     []string{"s"},
			Required:    false,
			Usage:       "Specify the url of a data source, in the format name=url. May be repeated to specify multiple sources. Postgres urls take the form 'postgres://username:password@hostname:5432/database_name'",
			Destination: &plotOpts.sources,
		},
		&cli.StringSliceFlag{
			Name:        "params",
			Aliases:     []string{"p"},
			Required:    false,
			Usage:       "Specify templating parameters, in the format key=value. May be repeated to specify multiple parameters.",
			Destination: &plotOpts.params,
		},
		&cli.StringFlag{
			Name:        "output",
			Aliases:     []string{"o"},
			Required:    false,
			Usage:       "Name of file JSON output should be written to. Output will be emitted to stdout by default.",
			Destination: &plotOpts.output,
		},
		&cli.StringFlag{
			Name:        "conf",
			Required:    false,
			Usage:       "Path of directory containing configuration.",
			Destination: &plotOpts.confDir,
		},
	}, loggingFlags...),
}

var plotOpts struct {
	preview  bool
	compact  bool
	sources  cli.StringSlice
	params   cli.StringSlice
	output   string
	validate bool
	confDir  string
}

func Plot(cc *cli.Context) error {
	ctx := cc.Context
	setupLogging()

	cfg := &PlotConfig{
		BasisTime: time.Now().UTC(),
		Sources: map[string]DataSource{
			"static": &StaticDataSource{},
			"demo":   &DemoDataSource{},
		},
		TemplateParams: map[string]any{},
	}

	for _, sopt := range plotOpts.sources.Value() {
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

	for _, param := range plotOpts.params.Value() {
		key, value, ok := strings.Cut(param, "=")
		if !ok {
			return fmt.Errorf("params option not valid, use format 'key=value'")
		}

		if _, exists := cfg.TemplateParams[key]; exists {
			return fmt.Errorf("duplicate template parameter %q specified", key)
		}

		cfg.TemplateParams[key] = value
	}

	if plotOpts.confDir != "" {
		conffs := os.DirFS(plotOpts.confDir)
		colorConfContent, err := fs.ReadFile(conffs, "colors.yaml")
		if err == nil {
			slog.Info("Parsing colors.yaml", "filename", path.Join(plotOpts.confDir, "colors.yaml"))
			var cd ColorDoc
			if err := yaml.Unmarshal(colorConfContent, &cd); err != nil {
				return fmt.Errorf("failed to unmarshal colors.yaml: %w", err)
			}
			cfg.DefaultColor = cd.Default
			cfg.Colors = make(map[string]string, len(cd.Colors))
			for _, nc := range cd.Colors {
				cfg.Colors[nc.Name] = nc.Color
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to read colors: %w", err)
		}
	}

	if cc.NArg() != 1 {
		return fmt.Errorf("plot definition must be supplied as an argument")
	}

	fname := cc.Args().Get(0)

	fcontent, err := os.ReadFile(fname)
	if err != nil {
		return fmt.Errorf("failed to read plot definition: %w", err)
	}

	templated, err := ExecuteTemplate(ctx, string(fcontent), cfg)
	if err != nil {
		return fmt.Errorf("failed to execute templates for plot definition: %w", err)
	}

	pd, err := parsePlotDef(fname, []byte(templated))
	if err != nil {
		return fmt.Errorf("failed to parse plot definition: %w", err)
	}

	if plotOpts.validate {
		fmt.Println("Name: " + pd.Name)
		fmt.Println("Frequency: " + pd.Frequency)
		fmt.Println("Datasets:")
		for _, ds := range pd.Datasets {
			fmt.Println("  Name: " + ds.Name)
			fmt.Println("  Source: " + ds.Source)
			fmt.Println("  Query:")
			fmt.Println(indent(ds.Query, "      "))

		}

		return nil
	}

	slog.Info("generating figure", "filename", fname)
	fig, err := generateFig(ctx, pd, cfg)
	if err != nil {
		return fmt.Errorf("failed to generate plot: %w", err)
	}

	figDat := FigureData{
		Fig:    fig,
		Params: pd.Parameters,
	}

	var data []byte
	if plotOpts.compact {
		data, err = json.Marshal(figDat)
	} else {
		data, err = json.MarshalIndent(figDat, "", "  ")
	}
	if err != nil {
		return fmt.Errorf("failed to marshal to json: %w", err)
	}

	var out io.Writer = os.Stdout
	if plotOpts.output != "" {
		f, err := os.Create(plotOpts.output)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		out = f
	}

	fmt.Fprintln(out, string(data))

	if plotOpts.preview {
		if err := preview(fig); err != nil {
			return fmt.Errorf("preview plot: %w", err)
		}
	}
	return nil
}

type DemoDataSource struct{}

func (s *DemoDataSource) GetDataSet(_ context.Context, query string, params ...any) (DataSet, error) {
	switch query {
	case "populations":
		return &StaticDataSet{Data: map[string][]any{
			"creature": {"giraffes", "orangutans", "monkeys"},
			"month1":   {20, 14, 23},
			"month2":   {2, 18, 29},
		}}, nil
	default:
		return nil, fmt.Errorf("unknown demo dataset: %s", query)
	}
}

func indent(s string, prefix string) string {
	s = strings.ReplaceAll(s, "\n", "\n"+prefix)
	return prefix + s
}

func plotname(fname string) string {
	base := filepath.Base(fname)
	return strings.TrimSuffix(base, filepath.Ext(fname))
}

func parsePlotDef(fname string, content []byte) (*PlotDef, error) {
	slog.Info("parsing plot definition file", "filename", fname)
	var pd PlotDef
	if err := yaml.Unmarshal(content, &pd); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plot definition: %w", err)
	}

	if pd.Name == "" {
		pd.Name = plotname(fname)
	}

	for _, s := range pd.Series {
		switch s.Type {
		case SeriesTypeBar, SeriesTypeHBar, SeriesTypeLine, SeriesTypeBox, SeriesTypeHBox:
		default:
			return nil, fmt.Errorf("unknown series type: %q", s.Type)
		}

		switch s.Fill {
		case FillTypeNone, FillTypeToZero:
		default:
			return nil, fmt.Errorf("unknown series fill: %q", s.Fill)
		}
	}

	for _, s := range pd.Scalars {
		switch s.Type {
		case ScalarTypeNumber:
		default:
			return nil, fmt.Errorf("unknown scalar type: %q", s.Type)
		}

		switch s.DeltaType {
		case DeltaTypeNone, DeltaTypeRelative:
		default:
			return nil, fmt.Errorf("unknown scalar delta type: %q", s.DeltaType)
		}
	}

	// annotate series with order in definition
	for i := range pd.Series {
		pd.Series[i].order = i
	}

	for _, t := range pd.Tables {
		switch t.Type {
		case TableTypeHeatmap:
		default:
			return nil, fmt.Errorf("unknown table type: %q", t.Type)
		}
	}

	// annotate series with order in definition
	for i := range pd.Tables {
		pd.Tables[i].order = i
	}

	return &pd, nil
}
