package paginator

import (
	"fmt"
	"slices"
	"sort"
	"torrentino/common/ordmap"
	"torrentino/common/utils"

	"github.com/pkg/errors"
)

type Evaluator interface {
	Compare(i int, j int, attribute string) bool
	Stringify(i int, attribute string) string
}

type Sorting struct {
	Attribute  string // attribute name in List.list[] items
	Alias      string // button text
	Order      int8   // 0 - unsorted, 1 - desc,  2 - asc
}

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

func (ls *List) Delete(i int) {
	idx := ls.index[i]
	ls.list = slices.Delete(ls.list, idx, idx+1) // ls.list = append(ls.list[:idx], ls.list[idx+1:]...)
	ls.Filter()                                  // <-- just for rebuild the indexes
}

func (ls *List) Item(i int) any {
	return ls.list[ls.index[i]]
}

func (ls *List) Stringify(item any, attributeName string) string {
	return ""
}

func (ls *List) Filter() {
	// defer utils.TimeTrack(utils.Now(), "Filtering")
	index := make([]int, 0, len(ls.list))
	ls.index = make([]int, len(ls.list))
	for i := range ls.list {
		ls.index[i] = i
	}
	for i := range ls.list {
		keepItem := ls.filters.Len() == 0
		for attribute, buttons := range ls.filters.Iter() {
			// stringValue := reflect.Indirect(reflect.ValueOf(ls.list[i])).FieldByName(attribute).String()
			value := ls.Evaluator.Stringify(i, attribute)
			keepItem = keepItem || buttons.GetUnsafe(value) || func() bool { //  exact filter on, or all filters is off
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
	ls.index = ls.index[:0] //ls.index = make([]int, 0, len(ls.list))
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

func (ls *List) ToggleFilter(attribute string, value string) {
	if buttons, ok := ls.filters.Get(attribute); ok {
		if enabled, ok := buttons.Get(value); ok {
			buttons.Set(value, !enabled)
		}
	}
}

func (ls *List) SetupSorting(attributes []Sorting) {
	ls.sorting.attributes = ordmap.New[string, *Sorting]()
	ls.sorting.queue = make([]string, 0, len(attributes))
	for _, attr := range attributes {
		ls.sorting.attributes.Set(attr.Attribute, &attr)
		if attr.Order != 0 {
			ls.sorting.queue = append(ls.sorting.queue, attr.Attribute)
		}
	}
}

func (ls *List) ToggleSorting(attribute string) {
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
		ls.sorting.queue = slices.Delete(ls.sorting.queue, idx, idx+1)
	}
}
