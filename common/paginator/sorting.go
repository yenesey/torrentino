package paginator

import (
	"fmt"
	"slices"
	"sort"
)

var sortChars = [...]string{"", "⏷", "⏶"}

type SortHeader struct {
	Name      string // attribute name in List items
	ShortName string // button text
	Order     int8   // 0 - unsorted, 1 - desc,  2 - asc
}

type SortingState struct {
	headers    []SortHeader
	multyOrder []int
}

func (s *SortingState) Setup(headers []SortHeader) {
	s.headers = headers
	s.multyOrder = make([]int, 0, len(s.headers))
	for i := range s.headers {
		if s.headers[i].Order != 0 {
			s.multyOrder = append(s.multyOrder, i)
		}
	}
}

func (s *SortingState) GetHeader(attributeKey string) (h *SortHeader, i int) {
	for i := range s.headers {
		if s.headers[i].Name == attributeKey {
			return &s.headers[i], i
		}
	}
	return nil, -1
}

func (s *SortingState) ToggleKey(attributeKey string) {

	var h, i = s.GetHeader(attributeKey)

	switch h.Order {
	case 0:
		h.Order = 1
		s.multyOrder = append(s.multyOrder, i)
	case 1:
		h.Order = 2
	case 2:
		h.Order = 0
		idx := slices.Index(s.multyOrder, i) // idx := slices.IndexFunc(s.multyOrder, func(el int) bool { return el == i })
		s.multyOrder = slices.Delete(s.multyOrder, idx, idx+1)
	}
	//s.SortingState()
	fmt.Println(s.multyOrder, s.headers)
}

// -------------------------------------------------------------
func (p *Paginator) Sort() {
	sort.Sort(p)
}

// part of sort.Interface
func (p *Paginator) Len() int {
	return len(p.index)
}

// part of sort.Interface
func (p *Paginator) Swap(i, j int) {
	p.index[i], p.index[j] = p.index[j], p.index[i]
}

// part of sort.Interface
func (p *Paginator) Less(i, j int) bool {
	s := &p.Sorting
	var k int
	for _, k = range s.multyOrder {
		h := &s.headers[k]
		switch {
		case p.virtual.LessItem(i, j, h.Name):
			return h.Order == 2

		case p.virtual.LessItem(j, i, h.Name):
			return h.Order != 2
		}
		// i == j; try the next comparison.
	}
	return false
}
