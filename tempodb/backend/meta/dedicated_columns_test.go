package meta

import (
	"errors"
	"testing"

	"github.com/grafana/tempo/pkg/tempopb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDedicatedColumnsFromTempopb(t *testing.T) {
	tests := []struct {
		name        string
		cols        []*tempopb.DedicatedColumn
		expected    DedicatedColumns
		expectedErr error
	}{
		{
			name: "no error",
			cols: []*tempopb.DedicatedColumn{
				{Scope: tempopb.DedicatedColumn_SPAN, Name: "test.span.1", Type: tempopb.DedicatedColumn_STRING},
				{Scope: tempopb.DedicatedColumn_RESOURCE, Name: "test.res.1", Type: tempopb.DedicatedColumn_STRING},
				{Scope: tempopb.DedicatedColumn_SPAN, Name: "test.span.2", Type: tempopb.DedicatedColumn_STRING},
			},
			expected: DedicatedColumns{
				{Scope: DedicatedColumnScopeSpan, Name: "test.span.1", Type: DedicatedColumnTypeString},
				{Scope: DedicatedColumnScopeResource, Name: "test.res.1", Type: DedicatedColumnTypeString},
				{Scope: DedicatedColumnScopeSpan, Name: "test.span.2", Type: DedicatedColumnTypeString},
			},
		},
		{
			name: "wrong type",
			cols: []*tempopb.DedicatedColumn{
				{Scope: tempopb.DedicatedColumn_RESOURCE, Name: "test.res.1", Type: tempopb.DedicatedColumn_Type(3)},
				{Scope: tempopb.DedicatedColumn_SPAN, Name: "test.span.2", Type: tempopb.DedicatedColumn_STRING},
			},
			expectedErr: errors.New("unable to convert dedicated column 'test.res.1': invalid value for tempopb.DedicatedColumn_Type '3'"),
		},
		{
			name: "wrong scope",
			cols: []*tempopb.DedicatedColumn{
				{Scope: tempopb.DedicatedColumn_RESOURCE, Name: "test.res.1", Type: tempopb.DedicatedColumn_STRING},
				{Scope: tempopb.DedicatedColumn_Scope(4), Name: "test.span.2", Type: tempopb.DedicatedColumn_STRING},
			},
			expectedErr: errors.New("unable to convert dedicated column 'test.span.2': invalid value for tempopb.DedicatedColumn_Scope '4'"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cols, err := DedicatedColumnsFromTempopb(tc.cols)
			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.EqualError(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, cols)
			}
		})
	}
}

func TestDedicatedColumns_ToTempopb(t *testing.T) {
	tests := []struct {
		name        string
		cols        DedicatedColumns
		expected    []*tempopb.DedicatedColumn
		expectedErr error
	}{
		{
			name: "no error",
			cols: DedicatedColumns{
				{Scope: DedicatedColumnScopeSpan, Name: "test.span.1", Type: DedicatedColumnTypeString},
				{Scope: DedicatedColumnScopeResource, Name: "test.res.1", Type: DedicatedColumnTypeString},
				{Scope: DedicatedColumnScopeSpan, Name: "test.span.2", Type: DedicatedColumnTypeString},
			},
			expected: []*tempopb.DedicatedColumn{
				{Scope: tempopb.DedicatedColumn_SPAN, Name: "test.span.1", Type: tempopb.DedicatedColumn_STRING},
				{Scope: tempopb.DedicatedColumn_RESOURCE, Name: "test.res.1", Type: tempopb.DedicatedColumn_STRING},
				{Scope: tempopb.DedicatedColumn_SPAN, Name: "test.span.2", Type: tempopb.DedicatedColumn_STRING},
			},
		},
		{
			name: "wrong type",
			cols: DedicatedColumns{
				{Scope: DedicatedColumnScopeSpan, Name: "test.span.1", Type: DedicatedColumnType("no-type")},
				{Scope: DedicatedColumnScopeResource, Name: "test.res.1", Type: DedicatedColumnTypeString},
			},
			expectedErr: errors.New("unable to convert dedicated column 'test.span.1': invalid value for dedicated column type 'no-type'"),
		},
		{
			name: "wrong scope",
			cols: DedicatedColumns{
				{Scope: DedicatedColumnScopeResource, Name: "test.res.1", Type: DedicatedColumnTypeString},
				{Scope: DedicatedColumnScope("no-scope"), Name: "test.span.2", Type: DedicatedColumnTypeString},
			},
			expectedErr: errors.New("unable to convert dedicated column 'test.span.2': invalid value for dedicated column scope 'no-scope'"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cols, err := tc.cols.ToTempopb()
			if tc.expectedErr != nil {
				require.Error(t, err)
				assert.EqualError(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, cols)
			}
		})
	}
}
