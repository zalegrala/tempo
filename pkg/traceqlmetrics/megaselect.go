package traceqlmetrics

import (
	"context"

	"github.com/grafana/tempo/pkg/traceql"
	"github.com/grafana/tempo/pkg/util"
	"github.com/pkg/errors"
)

func MegaSelect(ctx context.Context, query string, start, end uint64, fetcher traceql.SpansetFetcher) (*MetricsResults, error) {
	eval, req, err := traceql.NewEngine().Compile(query)
	if err != nil {
		return nil, errors.Wrap(err, "compiling query")
	}

	var (
		duration   = traceql.NewIntrinsic(traceql.IntrinsicDuration)
		name       = traceql.NewIntrinsic(traceql.IntrinsicName)
		startTime  = traceql.NewIntrinsic(traceql.IntrinsicSpanStartTime)
		startValue = traceql.NewStaticInt(int(start))
		status     = traceql.NewIntrinsic(traceql.IntrinsicStatus)
		// statusErr  = traceql.NewStaticStatus(traceql.StatusError)
		// spanCount  = 0
		// results    = NewMetricsResults()
	)

	if start > 0 {
		req.StartTimeUnixNanos = start
		req.Conditions = append(req.Conditions, traceql.Condition{Attribute: startTime, Op: traceql.OpGreaterEqual, Operands: []traceql.Static{startValue}})
	}
	if end > 0 {
		req.EndTimeUnixNanos = end
		// There is only an intrinsic for the span start time, so use it as the cutoff.
		req.Conditions = append(req.Conditions, traceql.Condition{Attribute: startTime, Op: traceql.OpLess, Operands: []traceql.Static{startValue}})
	}

	// Ensure that we select the span duration, status, and group-by attributes
	// in the second pass if they are not already part of the first pass.
	addConditionIfNotPresent := func(a traceql.Attribute) {
		for _, c := range req.Conditions {
			if c.Attribute == a {
				return
			}
		}

		req.SecondPassConditions = append(req.SecondPassConditions, traceql.Condition{Attribute: a})
	}
	addConditionIfNotPresent(status)
	addConditionIfNotPresent(duration)
	addConditionIfNotPresent(name)
	req.SecondPassConditions = append(req.SecondPassConditions, traceql.Condition{MegaSelect: true})

	req.SecondPass = func(s *traceql.Spanset) ([]*traceql.Spanset, error) {
		return eval([]*traceql.Spanset{s})
	}

	// Perform the fetch and process the results inside the SecondPass
	// callback.  No actual results will be returned from this fetch call,
	// But we still need to call Next() at least once.
	res, err := fetcher.Fetch(ctx, *req)
	if err == util.ErrUnsupported {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	defer res.Results.Close()

	results := NewMetricsResults()

	for {
		ss, err := res.Results.Next(ctx)
		if err != nil {
			return nil, err
		}
		if ss == nil {
			break
		}

		for _, s := range ss.Spans {
			// fmt.Println("Got span with attrs:")
			for k, v := range s.Attributes() {
				// fmt.Println("  ", k, v)
				if k != duration {
					results.Record(MetricSeries{KeyValue{Key: k.String(), Value: v}}, s.DurationNanos(), false)
				}
			}
		}

		ss.Release()
	}

	return results, nil
}
