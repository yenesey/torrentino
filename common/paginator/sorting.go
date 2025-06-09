package paginator

import (
	"slices"
)

var sortChars = [...]string{"", "▼", "▲"}

type SortHeader struct {
	AttributeName string // attribute name in List items
	ButtonText    string // button text
	Order         int8   // 0 - unsorted, 1 - desc,  2 - asc
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
		if s.headers[i].AttributeName == attributeKey {
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
		idx := slices.Index(s.multyOrder, i)
		s.multyOrder = slices.Delete(s.multyOrder, idx, idx+1)
	}
}
