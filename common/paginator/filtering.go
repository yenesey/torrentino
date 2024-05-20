package paginator

import (
	"reflect"
)

type FilteringHeader struct {
	Name    string
	Enabled bool
}

type FilteringAttribute struct {
	Name   string
	Values []FilteringHeader
	mmap   map[string]int
}

type FilteringState struct {
	attributes []FilteringAttribute
}

func (f *FilteringState) Setup(keys []string) {
	f.attributes = make([]FilteringAttribute, len(keys))
	for i, k := range keys {
		f.attributes[i].Name = k
		f.attributes[i].Values = make([]FilteringHeader, 0, 8)
		f.attributes[i].mmap = make(map[string]int)
	}
}

func (f *FilteringState) Get(attributeKey string, valueKey string) *FilteringHeader {
	for i := range f.attributes {
		for j := range f.attributes[i].Values {
			if (f.attributes[i].Name == attributeKey) &&
				(f.attributes[i].Values[j].Name == valueKey) {
				return &f.attributes[i].Values[j]
			}
		}
	}
	return nil
}

func (f *FilteringState) ClassifyItems(list []any) {
	for i := range f.attributes {
		countMap := make(map[string]bool) // count attributes currently present
		for j := range list {
			fieldValue := reflect.Indirect(reflect.ValueOf(list[j])).FieldByName(f.attributes[i].Name).String()
			if _, ok := f.attributes[i].mmap[fieldValue]; !ok {
				f.attributes[i].Values = append(f.attributes[i].Values, FilteringHeader{Name: fieldValue, Enabled: false})
				f.attributes[i].mmap[fieldValue] = len(f.attributes[i].Values) - 1
			}
			countMap[fieldValue] = true
		}
		for key := range f.attributes[i].mmap {
			if _, ok := countMap[key]; !ok {
				// remove filtering attributes that no more exists
				idx := f.attributes[i].mmap[key]
				f.attributes[i].Values = append(f.attributes[i].Values[:idx],f.attributes[i].Values[idx+1:]...)
				delete(f.attributes[i].mmap, key)
			}
		}
	}
}

// -------------------------------------------------------------

func (p *Paginator) Filter() {
	p.index = p.index[:0] //p.index = make([]int, 0, len(p.list))
	for li := range p.list {
		anyFilter := false
		KeepItem := false
		for i := range p.Filtering.attributes {
			for j := range p.Filtering.attributes[i].Values {
				enabled := p.Filtering.attributes[i].Values[j].Enabled
				if enabled {
					anyFilter = true
				}
				if enabled && p.virtual.KeepItem(p.list[li], p.Filtering.attributes[i].Name, p.Filtering.attributes[i].Values[j].Name) {
					KeepItem = true
				}
			}
		}
		if !anyFilter || (anyFilter && KeepItem) {
			p.index = append(p.index, li)
		}
	}
}