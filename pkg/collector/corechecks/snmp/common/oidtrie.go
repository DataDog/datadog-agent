package common

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strconv"
	"strings"
)

type OidTrie struct {
	Children map[int]*OidTrie
}

func newOidTrie() *OidTrie {
	return &OidTrie{}
}

func BuildTries(allOids []string) *OidTrie {
	root := newOidTrie()
	for _, oid := range allOids {
		current := root
		oid = strings.TrimLeft(oid, ".")
		segs := strings.Split(oid, ".")
		for _, seg := range segs {
			num, err := strconv.Atoi(seg)
			if err != nil {
				break
			}
			if current.Children == nil {
				current.Children = make(map[int]*OidTrie)
			}
			if _, ok := current.Children[num]; !ok {
				current.Children[num] = newOidTrie()
			}
			current = current.Children[num]
		}
	}
	return root
}

func (o *OidTrie) exist(oid string, isLeaf bool) bool {
	current := o
	oid = strings.TrimLeft(oid, ".")
	segs := strings.Split(oid, ".")
	for _, seg := range segs {
		num, err := strconv.Atoi(seg)
		if err != nil {
			return false
		}

		child, ok := current.Children[num]
		if !ok {
			return false
		}
		if len(child.Children) == 0 {
			return true
		}
		current = child
	}
	if isLeaf {
		return false
	}
	return true
}

func (o *OidTrie) NodeExist(oid string) bool {
	return o.exist(oid, false)
}

func (o *OidTrie) LeafExist(oid string) bool {
	return o.exist(oid, true)
}

func (o *OidTrie) Print(prefix string) {
	if len(o.Children) == 0 {
		log.Infof("OID: %s", prefix)
	}
	for oid, child := range o.Children {
		child.Print(fmt.Sprintf("%s.%d", prefix, oid))
	}
}
