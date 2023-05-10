package main

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/exp/slog"
)

type BinaryPredicate func(x, y any) (any, error)

type ComputeInput struct {
	Def     ComputeDataSetDef
	DataSet DataSet
}

func ComputeBinaryPredicate(ctx context.Context, pred BinaryPredicate, in1 ComputeInput, in2 ComputeInput) (DataSet, error) {
	in1.DataSet.ResetIterator()
	in2.DataSet.ResetIterator()

	rows2 := make(map[any]any)
	for in2.DataSet.Next() {
		join := in2.DataSet.Field(in2.Def.JoinField)
		if err, ok := join.(error); ok {
			return nil, fmt.Errorf("did not get join field value %q from dataset %q: %w", in2.Def.ValueField, in2.Def.DataSet, err)
		}
		value := in2.DataSet.Field(in2.Def.ValueField)
		if err, ok := value.(error); ok {
			return nil, fmt.Errorf("did not get value field value %q from dataset %q: %w", in2.Def.ValueField, in2.Def.DataSet, err)
		}
		rows2[stringify(join)] = value
	}
	if in2.DataSet.Err() != nil {
		return nil, fmt.Errorf("dataset iteration ended with an error: %w", in2.DataSet.Err())
	}

	data := make(map[string][]any)

	for in1.DataSet.Next() {

		join := in1.DataSet.Field(in1.Def.JoinField)
		if err, ok := join.(error); ok {
			return nil, fmt.Errorf("did not get join field value %q from dataset %q: %w", in1.Def.ValueField, in1.Def.DataSet, err)
		}

		value2, ok := rows2[stringify(join)]
		if !ok {
			slog.Debug("no matching row for join field", "join", join)
			continue
		}

		value1 := in1.DataSet.Field(in1.Def.ValueField)
		if err, ok := value1.(error); ok {
			return nil, fmt.Errorf("did not get value field value %q from dataset %q: %w", in1.Def.ValueField, in1.Def.DataSet, err)
		}

		res, err := pred(value1, value2)
		if err != nil {
			return nil, err
		}

		data["field"] = append(data["field"], join)
		data["value"] = append(data["value"], res)
	}
	if in1.DataSet.Err() != nil {
		return nil, fmt.Errorf("dataset iteration ended with an error: %w", in1.DataSet.Err())
	}

	return NewStaticDataSet(data), nil
}

func fieldValuesEqual(v1, v2 any) bool {
	switch tv1 := v1.(type) {
	case string:
		switch tv2 := v2.(type) {
		case string:
			return tv1 == tv2
		case time.Time:
			return tv1 == tv2.Format(time.RFC3339)
		}
	case time.Time:
		switch tv2 := v2.(type) {
		case string:
			return tv1.Format(time.RFC3339) == tv2
		case time.Time:
			return tv1.Equal(tv2)
		}
	}
	slog.Error("cannot compare field types", "type1", fmt.Sprintf("%T", v1), "type2", fmt.Sprintf("%T", v2))
	return false
}

func stringify(v any) string {
	switch tv := v.(type) {
	case string:
		return tv
	case time.Time:
		return tv.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprint(v)
	}
}

func diff2(x, y any) (any, error) {
	var diff any

	switch tx := x.(type) {
	case float64:
		switch ty := y.(type) {
		case float64:
			diff = tx - ty
		case int64:
			diff = tx - float64(ty)
		}
	case int64:
		switch ty := y.(type) {
		case int64:
			diff = tx - ty
		case int:
			diff = tx - int64(ty)
		case float64:
			diff = float64(tx) - ty
		}
	case int:
		switch ty := y.(type) {
		case int:
			diff = tx - ty
		case int64:
			diff = int64(tx) - ty
		case float64:
			diff = float64(tx) - ty
		}
	}

	if diff == nil {
		return nil, fmt.Errorf("cannot calculate diff of %T and %T", x, y)
	}

	return diff, nil
}
