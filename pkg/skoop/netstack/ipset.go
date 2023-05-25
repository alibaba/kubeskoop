package netstack

import (
	"github.com/beevik/etree"
	"github.com/pkg/errors"
)

type IPSetManager struct {
	sets map[string]*IPSet
}

func (m *IPSetManager) GetIPSet(name string) *IPSet {
	return m.sets[name]
}

type IPSet struct {
	Name    string            `json:"n"`
	Type    string            `json:"t"`
	Members map[string]string `json:"m"`
}

func NewIPSetManager(ipsets []*IPSet) (*IPSetManager, error) {
	ret := &IPSetManager{
		sets: make(map[string]*IPSet),
	}

	for _, i := range ipsets {
		ret.sets[i.Name] = i
	}
	return ret, nil
}

func ParseIPSet(dump string) ([]*IPSet, error) {
	var ret []*IPSet
	doc := etree.NewDocument()
	if err := doc.ReadFromString(dump); err != nil {
		return nil, err
	}
	for _, xmlIPSet := range doc.Root().ChildElements() {
		ipset, err := parseOneIPSet(xmlIPSet)
		if err != nil {
			return nil, errors.Wrap(err, "error parse ipset")
		}
		ret = append(ret, ipset)
	}
	return ret, nil
}

func parseOneIPSet(xmlIPSet *etree.Element) (*IPSet, error) {
	name := xmlIPSet.SelectAttr("name").Value
	_type := xmlIPSet.FindElement("type").Text()
	xmlMembers := xmlIPSet.FindElement("members")
	members := make(map[string]string)
	if xmlMembers != nil {
		for _, xmlMember := range xmlMembers.FindElements("member") {
			m := xmlMember.FindElement("elem").Text()
			members[m] = m
		}
	}

	return &IPSet{
		Name:    name,
		Type:    _type,
		Members: members,
	}, nil
}
