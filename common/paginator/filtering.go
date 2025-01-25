package paginator

import (
	// "torrentino/common/utils"
	"slices"
)

type FilterHeader struct {
	Value   string
	Enabled bool
}

type void struct{}

type FilterAttribute struct {
	AttributeName string
	Values        []FilterHeader
	valuesMap     map[string]void
}

type FilteringState struct {
	attributes []FilterAttribute
}

func (f *FilteringState) Setup(attributes []string) {
	f.attributes = make([]FilterAttribute, len(attributes))
	for i, AttributeName := range attributes {
		f.attributes[i].AttributeName = AttributeName
		f.attributes[i].Values = make([]FilterHeader, 0, 8)
		f.attributes[i].valuesMap = make(map[string]void)
	}
}

func (f *FilteringState) Get(attributeName string, value string) *FilterHeader {
	for i := range f.attributes {
		for j := range f.attributes[i].Values {
			if (f.attributes[i].AttributeName == attributeName) &&
				(f.attributes[i].Values[j].Value == value) {
				return &f.attributes[i].Values[j]
			}
		}
	}
	return nil
}

func (f *FilteringState) ClassifyItems(p VirtualMethods, l int) {
	// defer utils.TimeTrack(utils.Now(), "ClassifyItems")
	for j := range f.attributes {
		valuesNew := make(map[string]void) // count attributes currently present
		for i := 0; i < l; i++ {
			// fieldValue := reflect.Indirect(reflect.ValueOf(f.pg.list[j])).FieldByName(f.attributes[j].AttributeName).String()
			fieldValue := p.AttributeByName(i, f.attributes[j].AttributeName)
			if _, ok := f.attributes[j].valuesMap[fieldValue]; !ok {
				f.attributes[j].Values = append(f.attributes[j].Values, FilterHeader{Value: fieldValue, Enabled: false})
				f.attributes[j].valuesMap[fieldValue] = void{}
			}
			valuesNew[fieldValue] = void{}
		}
		for fieldValue := range f.attributes[j].valuesMap {
			if _, ok := valuesNew[fieldValue]; !ok {
				// remove filtering attributes that no more exists
				idx := slices.IndexFunc(f.attributes[j].Values, func(el FilterHeader) bool { return el.Value == fieldValue })
				f.attributes[j].Values = slices.Delete(f.attributes[j].Values, idx, idx+1)
				delete(f.attributes[j].valuesMap, fieldValue)
			}
		}
	}
}

// -------------------------------------------------------------

func (p *Paginator) Filter() {
	// defer utils.TimeTrack(utils.Now(), "Filtering")
	index := make([]int, 0, len(p.list))
	p.index = make([]int, len(p.list))
	for i := range p.list {
		p.index[i] = i
	}
	for i := range p.list {
		anyFilter := false
		keepItem := false
		for j := range p.Filtering.attributes {
			for k := range p.Filtering.attributes[j].Values {
				if p.Filtering.attributes[j].Values[k].Enabled {
					anyFilter = true
					if p.virtual.AttributeByName(i, p.Filtering.attributes[j].AttributeName) == p.Filtering.attributes[j].Values[k].Value {
						keepItem = true
					}
				}
			}
		}
		if !anyFilter || (anyFilter && keepItem) {
			index = append(index, i)
		}
	}
	p.index = p.index[:0] //p.index = make([]int, 0, len(p.list))
	p.index = append(p.index, index...)
}
