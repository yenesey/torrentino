// Package list is the UI-agnostic core of every Torrentino screen: items,
// sorting, filtering and pagination. It is shared by the Telegram paginator
// and the web interface so both use exactly the same rules.
package list

import (
	"fmt"
	"slices"
	"sort"

	"torrentino/common/ordmap"
	"torrentino/common/utils"

	"github.com/pkg/errors"
)

// Evaluator describes how items are compared and how their filter values are
// read. Indexes are positions in the current (filtered) view.
type Evaluator interface {
	Compare(i int, j int, attribute string) bool
	Stringify(i int, attribute string) string
}

// Sorting is one sortable attribute. Order is 0 (off), 1 (descending) or
// 2 (ascending); ToggleSorting cycles through those values.
type Sorting struct {
	Attribute string // attribute name passed to Evaluator
	Alias     string // button label
	Order     int8   // 0 - unsorted, 1 - desc, 2 - asc
}

// FilterValue is one selectable value of a filter attribute.
type FilterValue struct {
	Value   string
	Enabled bool
}

// List is the ordered, filterable and sortable collection of items. Its
// indexes point into the insertion order; sorting only permutes the indexes,
// so items themselves are never moved and their identity is stable.
type List struct {
	list  []any
	index []int
	Evaluator
	sorting struct {
		attributes *ordmap.OrderedMap[string, *Sorting]
		queue      []string
	}
	filters *ordmap.OrderedMap[string, *ordmap.OrderedMap[string, bool]]
}

func (ls *List) Alloc(l int) {
	ls.list = make([]any, 0, l)
	ls.index = make([]int, 0, l)
}

func (ls *List) Append(item any) {
	ls.list = append(ls.list, item)
	index := len(ls.list) - 1
	ls.index = append(ls.index, index)

	for attribute, buttons := range ls.filters.Iter() {
		value := ls.Evaluator.Stringify(index, attribute)
		if _, ok := buttons.Get(value); !ok {
			buttons.Set(value, false)
		}
	}
}

// Delete removes the item at the current view position. Out-of-range indexes
// are ignored: callers deal with stale state after a reload.
func (ls *List) Delete(i int) {
	if i < 0 || i >= ls.Len() {
		return
	}
	idx := ls.index[i]
	ls.list = slices.Delete(ls.list, idx, idx+1)
	ls.Filter() // rebuild the indexes
}

func (ls *List) Item(i int) any {
	return ls.list[ls.index[i]]
}

// ItemAt returns the item at the view position, or nil when the index is out
// of range. Web handlers prefer this over Item to avoid panics on stale input.
func (ls *List) ItemAt(i int) any {
	if i < 0 || i >= ls.Len() {
		return nil
	}
	return ls.Item(i)
}

func (ls *List) Filter() {
	index := make([]int, 0, len(ls.list))
	for i := range ls.list {
		keepItem := ls.filters.Len() == 0
		for attribute, buttons := range ls.filters.Iter() {
			value := ls.Evaluator.Stringify(i, attribute)
			keepItem = keepItem || buttons.GetUnsafe(value) || func() bool { // exact filter on, or all filters is off
				for _, enabled := range buttons.Iter() {
					if enabled {
						return false
					}
				}
				return true
			}()
		}
		if keepItem {
			index = append(index, i)
		}
	}
	ls.index = ls.index[:0]
	ls.index = append(ls.index, index...)
}

func (ls *List) Sort() {
	sort.Sort(ls)
}

// part of sort.Interface
func (ls *List) Len() int {
	return len(ls.index)
}

// part of sort.Interface
func (ls *List) Swap(i, j int) {
	ls.index[i], ls.index[j] = ls.index[j], ls.index[i]
}

// part of sort.Interface
func (ls *List) Less(i, j int) bool {
	for _, attribute := range ls.sorting.queue {
		order := ls.sorting.attributes.GetUnsafe(attribute).Order
		if ls.Evaluator.Compare(i, j, attribute) {
			return order == 2
		} else if ls.Evaluator.Compare(j, i, attribute) {
			return order != 2
		}
	}
	return false
}

func (ls *List) Compare(i int, j int, attributeName string) bool {
	return false
}

func (ls *List) SetupFiltering(attributes []string) {
	ls.filters = ordmap.New[string, *ordmap.OrderedMap[string, bool]]()
	for _, attr := range attributes {
		ls.filters.Set(attr, ordmap.New[string, bool]())
	}
}

// ToggleFilter switches a filter value. SetupFiltering is not called by every
// screen (e.g. /torrserver has no filters), so a forged callback for a filter
// that does not exist must be a no-op rather than a nil map access.
func (ls *List) ToggleFilter(attribute string, value string) {
	ls.SetFilter(attribute, value, !ls.filterEnabled(attribute, value))
}

// SetFilter sets one filter value explicitly and reports whether the
// attribute/value pair exists.
func (ls *List) SetFilter(attribute string, value string, enabled bool) bool {
	if ls.filters == nil {
		return false
	}
	buttons, ok := ls.filters.Get(attribute)
	if !ok {
		return false
	}
	if _, ok := buttons.Get(value); !ok {
		return false
	}
	buttons.Set(value, enabled)
	return true
}

func (ls *List) filterEnabled(attribute string, value string) bool {
	if ls.filters == nil {
		return false
	}
	buttons, ok := ls.filters.Get(attribute)
	if !ok {
		return false
	}
	enabled, _ := buttons.Get(value)
	return enabled
}

// FilterEnabled reports whether the given value is currently enabled.
func (ls *List) FilterEnabled(attribute string, value string) bool {
	return ls.filterEnabled(attribute, value)
}

// EnabledFilters returns all currently enabled values, grouped by attribute.
// It is used to rebuild filter options after a reload without losing the
// user's selection.
func (ls *List) EnabledFilters() map[string]map[string]bool {
	result := make(map[string]map[string]bool)
	for attribute, buttons := range ls.filters.Iter() {
		for value, enabled := range buttons.Iter() {
			if enabled {
				if result[attribute] == nil {
					result[attribute] = make(map[string]bool)
				}
				result[attribute][value] = true
			}
		}
	}
	return result
}

// ClearFilters disables every filter value.
func (ls *List) ClearFilters() {
	for _, buttons := range ls.filters.Iter() {
		for value := range buttons.Iter() {
			buttons.Set(value, false)
		}
	}
}

// FilterAttributes returns the configured filter attributes in order.
func (ls *List) FilterAttributes() []string {
	var result []string
	for attribute := range ls.filters.Iter() {
		result = append(result, attribute)
	}
	return result
}

// FilterValues returns the values of one filter attribute in the order they
// were first seen.
func (ls *List) FilterValues(attribute string) []FilterValue {
	if ls.filters == nil {
		return nil
	}
	buttons, ok := ls.filters.Get(attribute)
	if !ok {
		return nil
	}
	result := make([]FilterValue, 0, buttons.Len())
	for value, enabled := range buttons.Iter() {
		result = append(result, FilterValue{Value: value, Enabled: enabled})
	}
	return result
}

func (ls *List) SetupSorting(attributes []Sorting) {
	ls.sorting.attributes = ordmap.New[string, *Sorting]()
	ls.sorting.queue = make([]string, 0, len(attributes))
	for _, attr := range attributes {
		attr := attr
		ls.sorting.attributes.Set(attr.Attribute, &attr)
		if attr.Order != 0 {
			ls.sorting.queue = append(ls.sorting.queue, attr.Attribute)
		}
	}
}

// ToggleSorting advances one attribute through 1 -> 2 -> 0. A forged callback
// for an unknown attribute is a no-op rather than a nil dereference.
func (ls *List) ToggleSorting(attribute string) {
	if ls.sorting.attributes == nil {
		return
	}
	h, ok := ls.sorting.attributes.Get(attribute)
	if !ok {
		utils.LogError(errors.New(fmt.Sprintf("attribute %s not found", attribute)))
		return
	}
	h.Order += 1
	if h.Order == 1 {
		ls.sorting.queue = append(ls.sorting.queue, attribute)
	} else if h.Order > 2 {
		h.Order = 0
		idx := slices.Index(ls.sorting.queue, attribute)
		if idx >= 0 {
			ls.sorting.queue = slices.Delete(ls.sorting.queue, idx, idx+1)
		}
	}
}

// Sortings returns a copy of the configured sort attributes.
func (ls *List) Sortings() []Sorting {
	result := make([]Sorting, 0)
	for _, attr := range ls.sorting.attributes.Iter() {
		result = append(result, *attr)
	}
	return result
}

// SortingQueuePosition returns the 1-based position of an active attribute in
// the multi-sort queue, or 0 when it is not sorted by.
func (ls *List) SortingQueuePosition(attribute string) int {
	idx := slices.Index(ls.sorting.queue, attribute)
	if idx < 0 {
		return 0
	}
	return idx + 1
}

// HasSorting reports whether the attribute is configured.
func (ls *List) HasSorting(attribute string) bool {
	if ls.sorting.attributes == nil {
		return false
	}
	_, ok := ls.sorting.attributes.Get(attribute)
	return ok
}
