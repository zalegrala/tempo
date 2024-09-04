package meta

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"unsafe"

	"github.com/cespare/xxhash/v2"
	"github.com/grafana/tempo/pkg/tempopb"
	"github.com/grafana/tempo/pkg/traceql"
)

// DedicatedColumnType is the type of the values in the dedicated attribute column. Only 'string' is supported.
type DedicatedColumnType string

// DedicatedColumnScope is the scope of the attribute that is stored in a dedicated column. Possible values are
// 'resource' and 'span'.
type DedicatedColumnScope string

const (
	DedicatedColumnTypeString DedicatedColumnType = "string"

	DedicatedColumnScopeResource DedicatedColumnScope = "resource"
	DedicatedColumnScopeSpan     DedicatedColumnScope = "span"

	DefaultDedicatedColumnType  = DedicatedColumnTypeString
	DefaultDedicatedColumnScope = DedicatedColumnScopeSpan

	maxSupportedSpanColumns     = 10
	maxSupportedResourceColumns = 10
)

func DedicatedColumnTypeFromTempopb(t tempopb.DedicatedColumn_Type) (DedicatedColumnType, error) {
	switch t {
	case tempopb.DedicatedColumn_STRING:
		return DedicatedColumnTypeString, nil
	default:
		return "", fmt.Errorf("invalid value for tempopb.DedicatedColumn_Type '%v'", t)
	}
}

func (t DedicatedColumnType) ToTempopb() (tempopb.DedicatedColumn_Type, error) {
	switch t {
	case DedicatedColumnTypeString:
		return tempopb.DedicatedColumn_STRING, nil
	default:
		return 0, fmt.Errorf("invalid value for dedicated column type '%v'", t)
	}
}

func (t DedicatedColumnType) ToStaticType() (traceql.StaticType, error) {
	switch t {
	case DedicatedColumnTypeString:
		return traceql.TypeString, nil
	default:
		return traceql.TypeNil, fmt.Errorf("unsupported dedicated column type '%s'", t)
	}
}

func DedicatedColumnScopeFromTempopb(s tempopb.DedicatedColumn_Scope) (DedicatedColumnScope, error) {
	switch s {
	case tempopb.DedicatedColumn_SPAN:
		return DedicatedColumnScopeSpan, nil
	case tempopb.DedicatedColumn_RESOURCE:
		return DedicatedColumnScopeResource, nil
	default:
		return "", fmt.Errorf("invalid value for tempopb.DedicatedColumn_Scope '%v'", s)
	}
}

func (s DedicatedColumnScope) ToTempopb() (tempopb.DedicatedColumn_Scope, error) {
	switch s {
	case DedicatedColumnScopeSpan:
		return tempopb.DedicatedColumn_SPAN, nil
	case DedicatedColumnScopeResource:
		return tempopb.DedicatedColumn_RESOURCE, nil
	default:
		return 0, fmt.Errorf("invalid value for dedicated column scope '%v'", s)
	}
}

// DedicatedColumn contains the configuration for a single attribute with the given name that should
// be stored in a dedicated column instead of the generic attribute column.
type DedicatedColumn struct {
	// The Scope of the attribute
	Scope DedicatedColumnScope `yaml:"scope" json:"s,omitempty"`
	// The Name of the attribute stored in the dedicated column
	Name string `yaml:"name" json:"n"`
	// The Type of attribute value
	Type DedicatedColumnType `yaml:"type" json:"t,omitempty"`
}

func (dc *DedicatedColumn) MarshalJSON() ([]byte, error) {
	type dcAlias DedicatedColumn // alias required to avoid recursive calls of MarshalJSON

	cpy := (dcAlias)(*dc)
	if cpy.Scope == DefaultDedicatedColumnScope {
		cpy.Scope = ""
	}
	if cpy.Type == DefaultDedicatedColumnType {
		cpy.Type = ""
	}
	return json.Marshal(&cpy)
}

func (dc *DedicatedColumn) UnmarshalJSON(b []byte) error {
	type dcAlias DedicatedColumn // alias required to avoid recursive calls of UnmarshalJSON

	err := json.Unmarshal(b, (*dcAlias)(dc))
	if err != nil {
		return err
	}
	if dc.Scope == "" {
		dc.Scope = DefaultDedicatedColumnScope
	}
	if dc.Type == "" {
		dc.Type = DefaultDedicatedColumnType
	}
	return nil
}

func (dc DedicatedColumn) Marshal() ([]byte, error) {
	return dc.MarshalJSON()
}

func (dc *DedicatedColumn) MarshalTo(data []byte) (n int, err error) {
	bb, err := dc.Marshal()
	if err != nil {
		return 0, err
	}

	data = bytes.Clone(bb)
	return len(data), nil
}

func (dc *DedicatedColumn) Unmarshal(data []byte) error {
	return dc.UnmarshalJSON(data)
}

func (dc *DedicatedColumn) Size() int {
	return int(unsafe.Sizeof(*dc))
}

// DedicatedColumns represents a set of configured dedicated columns.
type DedicatedColumns []DedicatedColumn

func (dcs *DedicatedColumns) UnmarshalJSON(b []byte) error {
	// Get the pre-unmarshalled data if available.
	v := getDedicatedColumns(string(b))
	if v != nil {
		*dcs = *v
		return nil
	}

	type dcsAlias DedicatedColumns // alias required to avoid recursive calls of UnmarshalJSON
	err := json.Unmarshal(b, (*dcsAlias)(dcs))
	if err != nil {
		return err
	}

	putDedicatedColumns(string(b), dcs)

	return nil
}

func DedicatedColumnsFromTempopb(tempopbCols []*tempopb.DedicatedColumn) (DedicatedColumns, error) {
	cols := make(DedicatedColumns, 0, len(tempopbCols))

	for _, c := range tempopbCols {
		scope, err := DedicatedColumnScopeFromTempopb(c.Scope)
		if err != nil {
			return nil, fmt.Errorf("unable to convert dedicated column '%s': %w", c.Name, err)
		}

		typ, err := DedicatedColumnTypeFromTempopb(c.Type)
		if err != nil {
			return nil, fmt.Errorf("unable to convert dedicated column '%s': %w", c.Name, err)
		}

		cols = append(cols, DedicatedColumn{
			Scope: scope,
			Name:  c.Name,
			Type:  typ,
		})
	}

	return cols, nil
}

func (dcs DedicatedColumns) ToTempopb() ([]*tempopb.DedicatedColumn, error) {
	tempopbCols := make([]*tempopb.DedicatedColumn, 0, len(dcs))

	for _, c := range dcs {
		scope, err := c.Scope.ToTempopb()
		if err != nil {
			return nil, fmt.Errorf("unable to convert dedicated column '%s': %w", c.Name, err)
		}

		typ, err := c.Type.ToTempopb()
		if err != nil {
			return nil, fmt.Errorf("unable to convert dedicated column '%s': %w", c.Name, err)
		}

		tempopbCols = append(tempopbCols, &tempopb.DedicatedColumn{
			Scope: scope,
			Name:  c.Name,
			Type:  typ,
		})
	}

	return tempopbCols, nil
}

func (dcs DedicatedColumns) Validate() error {
	var countSpan, countRes int
	for _, dc := range dcs {
		err := dc.Validate()
		if err != nil {
			return err
		}
		switch dc.Scope {
		case DedicatedColumnScopeSpan:
			countSpan++
		case DedicatedColumnScopeResource:
			countRes++
		}
	}
	if countSpan > maxSupportedSpanColumns {
		return fmt.Errorf("number of dedicated columns with scope 'span' must be <= %d but was %d", maxSupportedSpanColumns, countSpan)
	}
	if countRes > maxSupportedResourceColumns {
		return fmt.Errorf("number of dedicated columns with scope 'resource' must be <= %d but was %d", maxSupportedResourceColumns, countRes)
	}
	return nil
}

func (dc *DedicatedColumn) Validate() error {
	if dc.Name == "" {
		return errors.New("dedicated column invalid: name must not be empty")
	}
	_, err := dc.Type.ToTempopb()
	if err != nil {
		return fmt.Errorf("dedicated column '%s' invalid: %w", dc.Name, err)
	}
	_, err = dc.Scope.ToTempopb()
	if err != nil {
		return fmt.Errorf("dedicated column '%s' invalid: %w", dc.Name, err)
	}
	return nil
}

// separatorByte is a byte that cannot occur in valid UTF-8 sequences
var separatorByte = []byte{255}

// Hash hashes the given dedicated columns configuration
func (dcs DedicatedColumns) Hash() uint64 {
	if len(dcs) == 0 {
		return 0
	}
	h := xxhash.New()
	for _, c := range dcs {
		_, _ = h.WriteString(string(c.Scope))
		_, _ = h.Write(separatorByte)
		_, _ = h.WriteString(c.Name)
		_, _ = h.Write(separatorByte)
		_, _ = h.WriteString(string(c.Type))
	}
	return h.Sum64()
}
