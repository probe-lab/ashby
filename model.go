package main

import (
	"context"
	"fmt"
	"time"

	grob "github.com/MetalBlueberry/go-plotly/graph_objects"
)

// PlotConfig provides external configuration and context to the generation
// of a plot.
type PlotConfig struct {
	// BasisTime is the time of execution of the queries in a plot
	// Generally it is the current time but can be set to a time in the past
	BasisTime time.Time

	// Sources is a mapping of names to datasources. The names can be
	// referenced in a dataset definition
	Sources map[string]DataSource

	// Template parameters can be provided on the command line. They
	// are passed directly to the templating engine.
	TemplateParams map[string]any

	DefaultColor string

	// Colors is a mapping of friendly names to hex values of colors
	Colors map[string]string
}

func (c *PlotConfig) MaybeLookupColor(name string, seriesName string) string {
	// if name == "" {
	// 	return c.DefaultColor
	// }
	v, ok := c.Colors[name]
	if ok {
		return v
	}
	return name
}

type PlotFrequency string

const (
	PlotFrequencyWeekly PlotFrequency = "weekly"
	PlotFrequencyDaily  PlotFrequency = "daily"
	PlotFrequencyHourly PlotFrequency = "hourly"
)

func (f PlotFrequency) String() string { return string(f) }

func (f PlotFrequency) Truncate(t time.Time) time.Time {
	switch f {
	case PlotFrequencyWeekly:
		return t.Truncate(7 * 24 * time.Hour)
	case PlotFrequencyDaily:
		return t.Truncate(24 * time.Hour)
	case PlotFrequencyHourly:
		return t.Truncate(time.Hour)
	default:
		panic(fmt.Sprintf("unsupported plot frequency: %q", f))
	}
}

type ProcessingProfile struct {
	Dir      string           `yaml:"directory"`
	OutTpl   string           `yaml:"output"`
	Variants []map[string]any `yaml:"variants"`
}

type PlotDef struct {
	Name      string        `yaml:"name"`
	Frequency PlotFrequency `yaml:"frequency"`
	Datasets  []DataSetDef  `yaml:"datasets"`
	Computed  []ComputedDef `yaml:"computed"`
	Series    []SeriesDef   `yaml:"series"`
	Scalars   []ScalarDef   `yaml:"scalars"`
	Layout    grob.Layout   `yaml:"layout"`
}

type DataSetDef struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"`
	Query  string `yaml:"query"`
}

type SeriesDef struct {
	Type       SeriesType `yaml:"type"`
	Name       string     `yaml:"name"` // name of the series
	Color      string     `yaml:"color"`
	Marker     MarkerType `yaml:"marker"`
	Fill       FillType   `yaml:"fill"`
	DataSet    string     `yaml:"dataset"`
	Labels     string     `yaml:"labels"`     // the name of the field the series should use for labels
	Values     string     `yaml:"values"`     // the name of the field the series should use for values
	GroupField string     `yaml:"groupfield"` // optional name of a field the series should use for grouping into related series
	GroupValue string     `yaml:"groupvalue"` // optional value of a field the series should use for grouping into related series
	Percent    bool       `yaml:"percent"`
	order      int        // used for retaining ordering of series
}

type SeriesType string

const (
	SeriesTypeBar  SeriesType = "bar"  // vertical bars
	SeriesTypeHBar SeriesType = "hbar" // horizontal bars
	SeriesTypeLine SeriesType = "line" // lines
	SeriesTypeBox  SeriesType = "box"  // vertical box plot
	SeriesTypeHBox SeriesType = "hbox" // horizontal box plot
)

func (t SeriesType) String() string { return string(t) }

type FillType string

const (
	FillTypeNone   FillType = ""
	FillTypeToZero FillType = "tozero"
)

func (t FillType) String() string { return string(t) }

type MarkerType string

const (
	// Note: this is only a subset of what plotly supports
	// see https://plotly.com/javascript/reference/scatter/#scatter-marker-symbol
	MarkerTypeNone     MarkerType = ""
	MarkerTypeCircle   MarkerType = "circle"
	MarkerTypeSquare   MarkerType = "square"
	MarkerTypeDiamond  MarkerType = "diamond"
	MarkerTypeTriangle MarkerType = "triangle"
	MarkerTypeHexagon  MarkerType = "hexagon"
)

func (t MarkerType) String() string { return string(t) }

type ScalarDef struct {
	Type          ScalarType `yaml:"type"`
	Name          string     `yaml:"name"` // name of the scalar
	Color         string     `yaml:"color"`
	DataSet       string     `yaml:"dataset"`
	Value         string     `yaml:"value"`         // the name of the field in the dataset that should be used for the scalar value
	ValueSuffix   string     `yaml:"valueSuffix"`   // a string to append after the value
	ValuePrefix   string     `yaml:"valuePrefix"`   // a string to prepend to the value
	DeltaDataSet  string     `yaml:"deltaDataset"`  // the name of a dataset to use for a delta value
	DeltaValue    string     `yaml:"deltaValue"`    // the name of the field in the delta dataset that should be used for the scalar value
	DeltaType     DeltaType  `yaml:"deltaType"`     // the type of delta contained in the value field
	IncreaseColor string     `yaml:"increaseColor"` // the color to use for delta that show an increase
	DecreaseColor string     `yaml:"decreaseColor"` // the color to use for delta that show an increase
}

type ScalarType string

const (
	ScalarTypeNumber ScalarType = "number" // display the scalar value as a number
)

func (t ScalarType) String() string { return string(t) }

type DeltaType string

const (
	DeltaTypeNone     DeltaType = ""
	DeltaTypeRelative DeltaType = "relative" // the delta is an absolute value and should be displayed with a relative % change to the scalar
)

func (t DeltaType) String() string { return string(t) }

type DataSource interface {
	GetDataSet(ctx context.Context, query string, params ...any) (DataSet, error)
}

type DataSeries struct {
	Labels []string
	Values []float64
}

type DataSet interface {
	Next() bool
	Err() error
	Field(name string) any
	ResetIterator()
}

// ColorDoc represents a document that defines a set of named colors
type ColorDoc struct {
	Default string       `yaml:"default"`
	Colors  []NamedColor `yaml:"colors"`
}

type NamedColor struct {
	Name  string `yaml:"name"`
	Color string `yaml:"color"`
}

// ComputedDef defines a computed dataset from a combination of others
type ComputedDef struct {
	Name     string              `yaml:"name"`
	Function ComputeType         `yaml:"function"`
	DataSets []ComputeDataSetDef `yaml:"datasets"`
}

type ComputeDataSetDef struct {
	DataSet    string `yaml:"dataset"`    // the name of the dataset
	JoinField  string `yaml:"joinField"`  // the field name that will be used to join the datasets
	ValueField string `yaml:"valueField"` // the field containing the value that will be used in the computation
}

type ComputeType string

const (
	ComputeTypeDiff ComputeType = "diff" // compute the difference between the first series and the second (first-second)
)

func (t ComputeType) String() string { return string(t) }
