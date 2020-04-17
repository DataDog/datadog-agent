package probe

type Cache struct{}

type SafeCache struct{}

func NewSafeCache() *SafeCache {
	return &SafeCache{}
}
