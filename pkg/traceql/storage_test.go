package traceql

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSpansetClone(t *testing.T) {
	ss := []*Spanset{
		{
			Spans: []Span{
				&mockSpan{
					id:                 []byte{0x01},
					startTimeUnixNanos: 3,
					durationNanos:      2,
				},
			},
			Scalar:             NewStaticFloat(3.2),
			TraceID:            []byte{0x02},
			RootSpanName:       "a",
			RootServiceName:    "b",
			StartTimeUnixNanos: 1,
			DurationNanos:      5,
			Attributes:         []*SpansetAttribute{{Name: "foo", Val: NewStaticString("bar")}},
		},
		{
			Spans: []Span{
				&mockSpan{
					id:                 []byte{0x01},
					startTimeUnixNanos: 3,
					durationNanos:      2,
				},
			},
			Scalar:             NewStaticFloat(3.2),
			TraceID:            []byte{0x02},
			RootSpanName:       "a",
			RootServiceName:    "b",
			StartTimeUnixNanos: 1,
			DurationNanos:      5,
		},
	}

	for _, s := range ss {
		require.True(t, reflect.DeepEqual(s, s.clone()))
	}
}

func TestMetaConditionsWithout(t *testing.T) {
	conditionsFor := func(q string) []Condition {
		req, err := ExtractFetchSpansRequest(q)
		require.NoError(t, err)

		return req.Conditions
	}

	tcs := []struct {
		remove []Condition
		expect []Condition
	}{
		{
			remove: []Condition{},
			expect: SearchMetaConditions(),
		},
		{
			remove: conditionsFor("{ duration > 1s}"),
			expect: []Condition{
				{NewIntrinsic(IntrinsicTraceRootService), OpNone, nil, false},
				{NewIntrinsic(IntrinsicTraceRootSpan), OpNone, nil, false},
				{NewIntrinsic(IntrinsicTraceDuration), OpNone, nil, false},
				{NewIntrinsic(IntrinsicTraceID), OpNone, nil, false},
				{NewIntrinsic(IntrinsicTraceStartTime), OpNone, nil, false},
				{NewIntrinsic(IntrinsicSpanID), OpNone, nil, false},
				{NewIntrinsic(IntrinsicSpanStartTime), OpNone, nil, false},
			},
		},
		{
			remove: conditionsFor("{ rootServiceName = `foo` && rootName = `bar`} | avg(duration) > 1s"),
			expect: []Condition{
				{NewIntrinsic(IntrinsicTraceDuration), OpNone, nil, false},
				{NewIntrinsic(IntrinsicTraceID), OpNone, nil, false},
				{NewIntrinsic(IntrinsicTraceStartTime), OpNone, nil, false},
				{NewIntrinsic(IntrinsicSpanID), OpNone, nil, false},
				{NewIntrinsic(IntrinsicSpanStartTime), OpNone, nil, false},
			},
		},
	}

	for _, tc := range tcs {
		require.Equal(t, tc.expect, SearchMetaConditionsWithout(tc.remove))
	}
}
