package paginator

import (
	// "torrentino/common/utils"
	"slices"
)

type FilteringHeader struct {
	Value   string
	Enabled bool
}

type void struct{}

type FilteringAttribute struct {
	Name      string
	Values    []FilteringHeader
	valuesMap map[string]void
}

type FilteringState struct {
	attributes []FilteringAttribute
	pg         *Paginator
}

func (f *FilteringState) Setup(attributes []string) {
	f.attributes = make([]FilteringAttribute, len(attributes))
	for i, attr := range attributes {
		f.attributes[i].Name = attr
		f.attributes[i].Values = make([]FilteringHeader, 0, 8)
		f.attributes[i].valuesMap = make(map[string]void)
	}
}

func (f *FilteringState) Get(attributeName string, value string) *FilteringHeader {
	for i := range f.attributes {
		for j := range f.attributes[i].Values {
			if (f.attributes[i].Name == attributeName) &&
				(f.attributes[i].Values[j].Value == value) {
				return &f.attributes[i].Values[j]
			}
		}
	}
	return nil
}

func (f *FilteringState) ClassifyItems() {
	// defer utils.TimeTrack(utils.Now(), "ClassifyItems")
	for j := range f.attributes {
		valuesNew := make(map[string]void) // count attributes currently present
		for i := range f.pg.list {
			// fieldValue := reflect.Indirect(reflect.ValueOf(f.pg.list[j])).FieldByName(f.attributes[j].Name).String()
			fieldValue := f.pg.virtual.AttributeByName(i, f.attributes[j].Name)
			if _, ok := f.attributes[j].valuesMap[fieldValue]; !ok {
				f.attributes[j].Values = append(f.attributes[j].Values, FilteringHeader{Value: fieldValue, Enabled: false})
				f.attributes[j].valuesMap[fieldValue] = void{}
			}
			valuesNew[fieldValue] = void{}
		}
		for fieldValue := range f.attributes[j].valuesMap {
			if _, ok := valuesNew[fieldValue]; !ok {
				// remove filtering attributes that no more exists
				idx := slices.IndexFunc(f.attributes[j].Values, func(el FilteringHeader) bool { return el.Value == fieldValue })
				f.attributes[j].Values = slices.Delete(f.attributes[j].Values, idx, idx+1)
				delete(f.attributes[j].valuesMap, fieldValue)
			}
		}
	}
}

// -------------------------------------------------------------

func (p *Paginator) Filter() {
	// defer utils.TimeTrack(utils.Now(), "Filtering")
	index := make([]int, 0, len(p.index))
	for i := range p.index {
		p.index[i] = i
	}
	for i := range p.list {
		anyFilter := false
		keepItem := false
		for j := range p.Filtering.attributes {
			for k := range p.Filtering.attributes[j].Values {
				if p.Filtering.attributes[j].Values[k].Enabled {
					anyFilter = true
					if p.virtual.AttributeByName(i, p.Filtering.attributes[j].Name) == p.Filtering.attributes[j].Values[k].Value {
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
