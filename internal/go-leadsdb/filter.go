package leadsdb

import (
	"fmt"
	"strings"
)

// SortField represents a field that can be used for sorting.
type SortField interface {
	sortFieldName() string
}

// Field represents a known lead field.
type Field string

func (f Field) sortFieldName() string { return string(f) }

// Known fields for type-safe sorting and filtering.
const (
	FieldName        Field = "name"
	FieldCity        Field = "city"
	FieldCountry     Field = "country"
	FieldState       Field = "state"
	FieldCategory    Field = "category"
	FieldSource      Field = "source"
	FieldEmail       Field = "email"
	FieldPhone       Field = "phone"
	FieldWebsite     Field = "website"
	FieldRating      Field = "rating"
	FieldReviewCount Field = "review_count"
	FieldCreatedAt   Field = "created_at"
	FieldUpdatedAt   Field = "updated_at"
)

// AttrSortField represents a custom attribute field for sorting.
type AttrSortField string

func (f AttrSortField) sortFieldName() string { return "attr:" + string(f) }

type logic string

const (
	logicAnd logic = "and"
	logicOr  logic = "or"
)

type filter struct {
	logic    logic
	operator string
	field    string
	value    string
}

func (f filter) String() string {
	if f.value == "" {
		return fmt.Sprintf("%s.%s.%s", f.logic, f.operator, f.field)
	}
	return fmt.Sprintf("%s.%s.%s.%s", f.logic, f.operator, f.field, f.value)
}

// FilterOption is a filter that can be passed to List.
type FilterOption struct {
	filter filter
}

func (f FilterOption) apply(cfg *listConfig) {
	cfg.filters = append(cfg.filters, f.filter)
}

// Or returns a builder for OR filters.
func Or() *OrBuilder {
	return &OrBuilder{}
}

// OrBuilder creates filters with OR logic.
type OrBuilder struct{}

func (b *OrBuilder) City() *TextField     { return &TextField{logic: logicOr, field: "city"} }
func (b *OrBuilder) Country() *TextField  { return &TextField{logic: logicOr, field: "country"} }
func (b *OrBuilder) State() *TextField    { return &TextField{logic: logicOr, field: "state"} }
func (b *OrBuilder) Name() *TextField     { return &TextField{logic: logicOr, field: "name"} }
func (b *OrBuilder) Email() *TextField    { return &TextField{logic: logicOr, field: "email"} }
func (b *OrBuilder) Phone() *TextField    { return &TextField{logic: logicOr, field: "phone"} }
func (b *OrBuilder) Website() *TextField  { return &TextField{logic: logicOr, field: "website"} }
func (b *OrBuilder) Category() *TextField { return &TextField{logic: logicOr, field: "category"} }
func (b *OrBuilder) Source() *TextField   { return &TextField{logic: logicOr, field: "source"} }
func (b *OrBuilder) Rating() *NumberField { return &NumberField{logic: logicOr, field: "rating"} }
func (b *OrBuilder) ReviewCount() *NumberField {
	return &NumberField{logic: logicOr, field: "review_count"}
}
func (b *OrBuilder) Tags() *ArrayField           { return &ArrayField{logic: logicOr, field: "tags"} }
func (b *OrBuilder) Location() *LocationField    { return &LocationField{logic: logicOr} }
func (b *OrBuilder) Attr(name string) *AttrField { return &AttrField{logic: logicOr, name: name} }

// AND filter starters (default)
func City() *TextField            { return &TextField{logic: logicAnd, field: "city"} }
func Country() *TextField         { return &TextField{logic: logicAnd, field: "country"} }
func State() *TextField           { return &TextField{logic: logicAnd, field: "state"} }
func Name() *TextField            { return &TextField{logic: logicAnd, field: "name"} }
func Email() *TextField           { return &TextField{logic: logicAnd, field: "email"} }
func Phone() *TextField           { return &TextField{logic: logicAnd, field: "phone"} }
func Website() *TextField         { return &TextField{logic: logicAnd, field: "website"} }
func Category() *TextField        { return &TextField{logic: logicAnd, field: "category"} }
func Source() *TextField          { return &TextField{logic: logicAnd, field: "source"} }
func Rating() *NumberField        { return &NumberField{logic: logicAnd, field: "rating"} }
func ReviewCount() *NumberField   { return &NumberField{logic: logicAnd, field: "review_count"} }
func Tags() *ArrayField           { return &ArrayField{logic: logicAnd, field: "tags"} }
func Location() *LocationField    { return &LocationField{logic: logicAnd} }
func Attr(name string) *AttrField { return &AttrField{logic: logicAnd, name: name} }

// TextField for text field filters.
type TextField struct {
	logic logic
	field string
}

func (f *TextField) Eq(value string) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "eq", field: f.field, value: value}}
}

func (f *TextField) Neq(value string) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "neq", field: f.field, value: value}}
}

func (f *TextField) Contains(value string) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "contains", field: f.field, value: value}}
}

func (f *TextField) NotContains(value string) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "not_contains", field: f.field, value: value}}
}

func (f *TextField) IsEmpty() FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "is_empty", field: f.field}}
}

func (f *TextField) IsNotEmpty() FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "is_not_empty", field: f.field}}
}

// NumberField for numeric field filters.
type NumberField struct {
	logic logic
	field string
}

func (f *NumberField) Eq(value float64) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "eq", field: f.field, value: formatNumber(value)}}
}

func (f *NumberField) Neq(value float64) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "neq", field: f.field, value: formatNumber(value)}}
}

func (f *NumberField) Gt(value float64) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "gt", field: f.field, value: formatNumber(value)}}
}

func (f *NumberField) Gte(value float64) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "gte", field: f.field, value: formatNumber(value)}}
}

func (f *NumberField) Lt(value float64) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "lt", field: f.field, value: formatNumber(value)}}
}

func (f *NumberField) Lte(value float64) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "lte", field: f.field, value: formatNumber(value)}}
}

// ArrayField for array field filters (e.g., tags).
type ArrayField struct {
	logic logic
	field string
}

func (f *ArrayField) Contains(value string) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "array_contains", field: f.field, value: value}}
}

func (f *ArrayField) NotContains(value string) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "array_not_contains", field: f.field, value: value}}
}

func (f *ArrayField) IsEmpty() FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "array_empty", field: f.field}}
}

func (f *ArrayField) IsNotEmpty() FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "array_not_empty", field: f.field}}
}

// LocationField for location-based filters.
type LocationField struct {
	logic logic
}

func (f *LocationField) WithinRadius(lat, lon, km float64) FilterOption {
	value := fmt.Sprintf("%s,%s,%s", formatNumber(lat), formatNumber(lon), formatNumber(km))
	return FilterOption{filter{logic: f.logic, operator: "within_radius", field: "location", value: value}}
}

func (f *LocationField) IsSet() FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "is_set", field: "location"}}
}

func (f *LocationField) IsNotSet() FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "is_not_set", field: "location"}}
}

// AttrField for custom attribute filters.
type AttrField struct {
	logic logic
	name  string
}

func (f *AttrField) field() string {
	return "attr:" + f.name
}

func (f *AttrField) Eq(value string) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "eq", field: f.field(), value: value}}
}

func (f *AttrField) Neq(value string) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "neq", field: f.field(), value: value}}
}

func (f *AttrField) Contains(value string) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "contains", field: f.field(), value: value}}
}

func (f *AttrField) EqNumber(value float64) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "eq", field: f.field(), value: formatNumber(value)}}
}

func (f *AttrField) Gt(value float64) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "gt", field: f.field(), value: formatNumber(value)}}
}

func (f *AttrField) Gte(value float64) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "gte", field: f.field(), value: formatNumber(value)}}
}

func (f *AttrField) Lt(value float64) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "lt", field: f.field(), value: formatNumber(value)}}
}

func (f *AttrField) Lte(value float64) FilterOption {
	return FilterOption{filter{logic: f.logic, operator: "lte", field: f.field(), value: formatNumber(value)}}
}

func formatNumber(v float64) string {
	s := fmt.Sprintf("%g", v)
	return strings.TrimSuffix(s, ".0")
}
