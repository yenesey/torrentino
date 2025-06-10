package paginator

import (
	"slices"
	"sort"
)

type Evaluator interface {
	Compare(i int, j int, attribute string) bool
	Stringify(i int, attribute string) string
}

type List struct {
	list  []any
	index []int
	Evaluator
	Sorting
	Filtering
}

func (ls *List) Alloc(l int) {
	ls.list = make([]any, 0, l)
	ls.index = make([]int, 0, l)
}

func (ls *List) Append(item any) {
	ls.list = append(ls.list, item)
	index := len(ls.list)-1
	ls.index = append(ls.index, index)

	for i := range ls.Filtering.attributes {
		attr := &ls.Filtering.attributes[i]
		value := ls.Evaluator.Stringify(index, attr.Attribute)
		if _, ok := attr.States[value]; !ok {
			attr.States[value] = false
			attr.Values = append(attr.Values, value)
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
		keepItem := len(ls.Filtering.attributes) == 0
		for j := range ls.Filtering.attributes {
			attr := &ls.Filtering.attributes[j]
			// stringValue := reflect.Indirect(reflect.ValueOf(ls.list[i])).FieldByName(attr.Attribute).String()
			stringValue := ls.Evaluator.Stringify(i, attr.Attribute)
			keepItem = keepItem || attr.States[stringValue] || func() bool { //  exact filter on, or all filters is off
				for _, state := range attr.States {
					if state {
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

	for _, k := range ls.Sorting.queue {
		h := &ls.Sorting.headers[k]
		switch {
		case ls.Evaluator.Compare(i, j, h.Attribute):
			return h.Order == 2

		case ls.Evaluator.Compare(j, i, h.Attribute):
			return h.Order != 2
		}
		// i == j; try the next comparison.
	}
	return false
}

func (ls *List) Compare(i int, j int, attributeName string) bool {
	return false
}
