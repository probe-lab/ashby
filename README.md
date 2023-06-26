# ashby

`ashby` produces plots for the [ProbeLab CMI website](https://github.com/plprobelab/website)

Currently it only supports an interactive mode but the goal is to run it as a daemon and for it to 
produce automatically plots on a schedule.

## Getting Started

Run `go build` in the `/cmd/ashby` folder.

If you're just trying the static demo plots then you can use:

	./ashby plot --preview ../../plots/demo-static-bar-grouped.json

It outputs the JSON that defines the plot and can be passed to the plotly JavaScript library.

`--preview` makes ashby launch the plot as a preview in your browser

If you want to try a postgres demo plot then you need to specify the postgres datasource with the `-s` option:

	-s name=postgres://username:password@hostname:5432/database_name

For the demos the name is `pgnebula`, so you should run:

	./ashby plot --preview -s pgnebula=postgres://username:password@hostname/database_name?sslmode=prefer ../../plots/demo-pg-agents-bar.json

(with the database details filled in, of course!)


## Plot Specifications

Plots are defined in JSON. Some samples are in the `plots` folder at the root of this repo.

The specification format is in flux but currently there are three sections to each plot specification.

 - `datasets` - this specifies a list of named datasets that provide source data for the plots. Each has a source and query. Support for query parameters is on the TODO list. A dataset is essentially a list of named fields and their data values, usually a tabular structure.
 - `series` - this specifies a list of series that are to be plotted. Each series specifies the field to use for labelling the points in the series and a field for the values. Each series will be plotted onto the final chart.
 - `layout` - this defines the layout for the plot. Currently it's just the same as the plotly layout definition but ideally we will support only a useful subset to avoid coupling too tightly to a single plotting library.


An example plot spec:

```json
	{
	  "datasets": [
	    {
	      "name":"main",
	      "source":"demo",
	      "query":"populations"
	    }
	  ],

	  "series": [
	    {
	      "type": "bar",
	      "name": "population",
	      "dataset": "main",
	      "labels": "creature",
	      "values": "month1"
	    }
	  ],

	  "layout":{
	    "title":{
	      "text":"Demo: Bar"
	    }
	  }
	}
```

## Templating

Plot definitions may use Go's templating capabilities. 
Plot definition files are parsed and executed as a Go template before parsing into the final plot definition model.

The following functions are available:

 - All [Sprig](https://masterminds.github.io/sprig/) functions 
 - `timestamptz` - format a time as a Postgresql `timestamptz`  (for example: `'2023-03-13 00:00:00 Z'::timestampz`)
 - `timestamp` - format a time as a Postgresql `timestamp`  (for example: `'2023-03-13 00:00:00'::timestamp`)
 - `simpledate` - format a time in a simple, human readable format (for example: `13 Mar 2023`)
 - `isodate` - format a time as RFC3339 (for example: `2023-03-13T00:00:00Z`)
 - `dayModify` - a version of [sprig's dateModify](https://masterminds.github.io/sprig/date.html#datemodify-mustdatemodify) that accepts a number of days
 - `weekModify` - a version of [sprig's dateModify](https://masterminds.github.io/sprig/date.html#datemodify-mustdatemodify) that accepts a number of weeks
 - `monthModify` - a version of [sprig's dateModify](https://masterminds.github.io/sprig/date.html#datemodify-mustdatemodify) that accepts a number of months

The following data variables are available:

 - `.Now` - the basis time for the plot, this might be the current date or a date in the past if the plot is being regenerated. This should be used as the basis for all date calculations
 - `.StartOfHour` - the basis time truncated to the hour, so that minutes and seconds are removed
 - `.StartOfDay` - the basis time truncated to the day, so that hours, minutes and seconds are removed
 - `.StartOfWeek` - the basis time truncated to the start of the week containing the basis time

The following are useful when formatting dates that are immediately before the start of the period.
They are not really suitable for use as the end of a range in a query.

 - `.EndOfPreviousHour` - one nanosecond before `.StartOfHour`
 - `.EndOfPreviousHour` - one nanosecond before `.StartOfDay`
 - `.EndOfPreviousHour` - one nanosecond before `.StartOfWeek`


### Templating Examples

A one week range up to the week including the basis time, in Postgresql:

	tstzrange({{ .StartOfWeek | timestamptz }}-'1 week'::interval, {{ .StartOfWeek | timestamptz }})

Limiting a Postgresql query to 30 days before the basis time:

	and m1.created_at >= {{ .StartOfDay | timestamptz }}-'30 day'::interval
	and m1.created_at < {{ .StartOfDay | timestamptz }}

Using `.EndOfPreviousDay` to construct a title for a plot:

	30 days up to {{ .EndOfPreviousDay | simpledate }}
