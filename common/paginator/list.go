package paginator

import (
	"slices"
	"sort"
)

type Comparator interface {
	Compare(i int, j int, attributeName string) bool
	Value(item any, attributeName string) string
}

type List struct {
	list  []any
	index []int
	Comparator
	Sorting   SortingState
	Filtering FilteringState
}

func (ls *List) Alloc(l int) {
	ls.list = make([]any, 0, l)
	ls.index = make([]int, 0, l)
}

func (ls *List) Append(item any) {
	ls.list = append(ls.list, item)
	ls.index = append(ls.index, len(ls.list)-1)

	for i := range ls.Filtering.attributes {
		attr := &ls.Filtering.attributes[i]
		value := ls.Comparator.Value(item, attr.AttributeName)
		if _, ok := attr.State[value]; !ok {
			attr.State[value] = false
			attr.Values = append(attr.Values, value)
		}
	}
}

func (ls *List) Delete(i int) {
	idx := ls.index[i]
	ls.list = slices.Delete(ls.list, idx, idx+1) // ls.list = append(ls.list[:idx], ls.list[idx+1:]...)
	ls.Filter()                                  // <-- just for rebuild the indexes
}

func (ls *List) Value(item any, attributeName string) string {
	return ""
}

func (ls *List) Filter() {
	// defer utils.TimeTrack(utils.Now(), "Filtering")
	index := make([]int, 0, len(ls.list))
	for i := range ls.list {
		keepItem := len(ls.Filtering.attributes) == 0
		for j := range ls.Filtering.attributes {
			attr := &ls.Filtering.attributes[j]
			// stringValue := reflect.Indirect(reflect.ValueOf(ls.list[i])).FieldByName(attr.AttributeName).String()
			stringValue := ls.Comparator.Value(ls.list[i], attr.AttributeName)
			keepItem = keepItem || attr.State[stringValue] || func() bool { //  exact filter on, or all filters is off
				for _, state := range attr.State {
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
	s := &ls.Sorting
	var k int
	for _, k = range s.multyOrder {
		h := &s.headers[k]
		switch {
		case ls.Comparator.Compare(i, j, h.AttributeName):
			return h.Order == 2

		case ls.Comparator.Compare(j, i, h.AttributeName):
			return h.Order != 2
		}
		// i == j; try the next comparison.
	}
	return false
}

func (ls *List) Compare(i int, j int, attributeName string) bool {
	return false
}
