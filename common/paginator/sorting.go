package paginator

import (
	"slices"
)

var sortChars = [...]string{"", "▼", "▲"}

type SortingHeader struct {
	Attribute  string // attribute name in List items
	ButtonText string // button text
	Order      int8   // 0 - unsorted, 1 - desc,  2 - asc
}

type Sorting struct {
	headers []SortingHeader
	queue []int
}

func (s *Sorting) Setup(headers []SortingHeader) {
	s.headers = headers
	s.queue = make([]int, 0, len(s.headers))
	for i := range s.headers {
		if s.headers[i].Order != 0 {
			s.queue = append(s.queue, i)
		}
	}
}

func (s *Sorting) GetHeader(attribute string) (i int, h *SortingHeader) {
	for i := range s.headers {
		if s.headers[i].Attribute == attribute {
			return i, &s.headers[i]
		}
	}
	return -1, nil
}

func (s *Sorting) ToggleAttribute(attribute string) {

	var i, h = s.GetHeader(attribute)
	switch h.Order {
	case 0:
		h.Order = 1
		s.queue = append(s.queue, i)
	case 1:
		h.Order = 2
	case 2:
		h.Order = 0
		idx := slices.Index(s.queue, i)
		s.queue = slices.Delete(s.queue, idx, idx+1)
	}
}
