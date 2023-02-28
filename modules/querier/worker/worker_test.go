package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
)

func BenchmarkQuerierWorker(b *testing.B) {
	w, err := NewQuerierWorker(Config{}, nil, log.NewNopLogger())
	require.NoError(b, err)
	err = w.StartAsync(context.TODO())
	require.NoError(b, err)

	fmt.Println("Starting benchmark")

	for i := 0; i < b.N; i++ {
		w.(*querierWorker).AddressAdded(fmt.Sprintf("test%d:1111", i))
	}
}
