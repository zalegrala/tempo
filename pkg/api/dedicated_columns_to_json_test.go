package api

import (
	"encoding/json"
	"math/rand/v2"
	"testing"

	"github.com/grafana/tempo/pkg/util/test"
	"github.com/grafana/tempo/tempodb/backend/meta"
	"github.com/stretchr/testify/require"
)

func TestDedicatedColumnsToJson(t *testing.T) {
	d := NewDedicatedColumnsToJSON()

	testCols := []meta.DedicatedColumns{}
	for i := 0; i < 10; i++ {
		testCols = append(testCols, randoDedicatedCols())
	}

	// do all test cols 2x to test caching
	for i := 0; i < 2; i++ {
		for _, cols := range testCols {
			expectedJSON := dedicatedColsToJSON(t, cols)
			actualJSON, err := d.JSONForDedicatedColumns(cols)
			require.NoError(t, err)

			require.Equal(t, expectedJSON, actualJSON, "iteration %d, cols: %v", i, cols)
		}
	}
}

func dedicatedColsToJSON(t *testing.T, cols meta.DedicatedColumns) string {
	t.Helper()

	proto, err := cols.ToTempopb()
	require.NoError(t, err)

	jsonBytes, err := json.Marshal(proto)
	require.NoError(t, err)

	return string(jsonBytes)
}

// randoDedicatedCols generates a random set of cols for testing
func randoDedicatedCols() meta.DedicatedColumns {
	colCount := rand.IntN(5) + 1
	ret := make([]meta.DedicatedColumn, 0, colCount)

	for i := 0; i < colCount; i++ {
		scope := meta.DedicatedColumnScopeSpan
		if rand.IntN(2) == 0 {
			scope = meta.DedicatedColumnScopeResource
		}

		col := meta.DedicatedColumn{
			Scope: scope,
			Name:  test.RandomString(),
			Type:  meta.DedicatedColumnTypeString,
		}

		ret = append(ret, col)
	}

	return ret
}
