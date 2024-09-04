package ingester

import (
	"github.com/grafana/tempo/modules/generator/registry"
	"github.com/grafana/tempo/modules/overrides"
	"github.com/grafana/tempo/tempodb/backend/meta"
)

type ingesterOverrides interface {
	registry.Overrides

	DedicatedColumns(userID string) meta.DedicatedColumns
}

var _ ingesterOverrides = (overrides.Interface)(nil)
