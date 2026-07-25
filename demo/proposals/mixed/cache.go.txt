package demo

type Cache struct {
	values map[string]string
}

func newCache() *Cache {
	return &Cache{}
}

func (c *Cache) Put(key, value string) {
	c.values[key] = value
}
