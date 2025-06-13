package paginator

import (
	"slices"
	"sort"
	"torrentino/common/ordmap"
)

type Evaluator interface {
	Compare(i int, j int, attribute string) bool
	Stringify(i int, attribute string) string
}

type Sorting struct {
	Attribute  string // attribute name in List.list[] items
	ButtonText string // button text
	Order      int8   // 0 - unsorted, 1 - desc,  2 - asc
}

type List struct {
	list  []any
	index []int
	Evaluator
	sorting struct {
		attributes []Sorting
		queue      []int
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
			keepItem = keepItem || buttons.GetOne(value) || func() bool { //  exact filter on, or all filters is off
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

	for _, k := range ls.sorting.queue {
		attr := &ls.sorting.attributes[k]
		switch {
		case ls.Evaluator.Compare(i, j, attr.Attribute):
			return attr.Order == 2

		case ls.Evaluator.Compare(j, i, attr.Attribute):
			return attr.Order != 2
		}
		// i == j; try the next comparison.
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
	ls.sorting.attributes = attributes
	ls.sorting.queue = make([]int, 0, len(attributes))
	for i := range ls.sorting.attributes {
		if ls.sorting.attributes[i].Order != 0 {
			ls.sorting.queue = append(ls.sorting.queue, i)
		}
	}
}

func (ls *List) getSortingHeader(attribute string) (i int, h *Sorting) {
	for i := range ls.sorting.attributes {
		if ls.sorting.attributes[i].Attribute == attribute {
			return i, &ls.sorting.attributes[i]
		}
	}
	return -1, nil
}

func (ls *List) ToggleSorting(attribute string) {
	i, h := ls.getSortingHeader(attribute)
	switch h.Order {
	case 0:
		h.Order = 1
		ls.sorting.queue = append(ls.sorting.queue, i)
	case 1:
		h.Order = 2
	case 2:
		h.Order = 0
		idx := slices.Index(ls.sorting.queue, i)
		ls.sorting.queue = slices.Delete(ls.sorting.queue, idx, idx+1)
	}
}
