package paginator

import (
	// "reflect"
	// "torrentino/common/utils"
)

type FilterAttribute struct {
	AttributeName string
	Values        []string
	State         map[string]bool
}

type FilteringState struct {
	attributes []FilterAttribute
}

func (f *FilteringState) Setup(attributes []string) {
	f.attributes = make([]FilterAttribute, len(attributes))
	for i, AttributeName := range attributes {
		f.attributes[i].AttributeName = AttributeName
		f.attributes[i].Values = make([]string, 0, 8)
		f.attributes[i].State = make(map[string]bool)
	}
}

func (f *FilteringState) Toggle(attributeName string, attributeValue string) {
	for i := range f.attributes {
		if f.attributes[i].AttributeName == attributeName {
			f.attributes[i].State[attributeValue] = !f.attributes[i].State[attributeValue]
			break
		}
	}
}

// -------------------------------------------------------------

func (p *Paginator) Filter() {
	// defer utils.TimeTrack(utils.Now(), "Filtering")
	index := make([]int, 0, len(p.list))
	for i := range p.list {	
		keepItem := len(p.Filtering.attributes) == 0
		for j := range p.Filtering.attributes {
			attr := &p.Filtering.attributes[j]
			// stringValue := reflect.Indirect(reflect.ValueOf(p.list[i])).FieldByName(attr.AttributeName).String()
			stringValue := p.virtual.StringValueByName(p.list[i], attr.AttributeName)
			keepItem = keepItem || attr.State[stringValue] || func() bool { //  exact filter on, or all filters is off
				for _, state := range attr.State {
					if state { return false }
				}
				return true
			}() 
		}
		if keepItem {
			index = append(index, i)
		}
	}
	p.index = p.index[:0] //p.index = make([]int, 0, len(p.list))
	p.index = append(p.index, index...)
}
