package pack

import "strconv"

type TagsPackerInterface interface {
	Pack(tags []string, consumer StringConsumer) error
}

type TagsPacker struct {
	tags []string
}

type TagsUnpacker struct {
	tags []string
}

type StringConsumer func(s string) error

func NewTagsPacker() *TagsPacker {
	return &TagsPacker{}
}

func NewTagsUnpacker() *TagsUnpacker {
	return &TagsUnpacker{}
}

func (p *TagsPacker) Pack(tags []string, consumer StringConsumer) error {
	i, j := 0, 0

	// Compare the tags and emit ranges of matching tags
	for i < len(p.tags) && j < len(tags) {
		n := findNumberOfMatchingTags(p.tags[i:], tags[j:])
		if n != 0 {
			err := consumer(offsetPrefixStr + strconv.FormatInt(int64(i), 16) + "-" + strconv.FormatInt(int64(n), 16))
			if err != nil {
				return err
			}
			i += n
			j += n
		} else {
			err := consumer(tags[j])
			if err != nil {
				return err
			}
			j++
			i++
		}
	}

	// All the remaining tags are new
	for ; j < len(tags); j++ {
		err := consumer(tags[j])
		if err != nil {
			return err
		}
	}

	p.tags = tags
	return nil
}

func findNumberOfMatchingTags(a []string, b []string) int {
	n := 0

	for i, j := 0, 0; i < len(a) && j < len(b); {
		if a[i] != b[j] {
			break
		}
		n++
		i++
		j++
	}
	return n
}

func (u *TagsUnpacker) UnPack(tags []string, consumer StringConsumer) {

}

type NoopTagsPacker struct{}

func (p *NoopTagsPacker) Pack(tags []string, consumer StringConsumer) error {
	for _, tag := range tags {
		err := consumer(tag)
		if err != nil {
			return err
		}
	}
	return nil
}

var _ TagsPackerInterface = &TagsPacker{}
var _ TagsPackerInterface = &NoopTagsPacker{}
