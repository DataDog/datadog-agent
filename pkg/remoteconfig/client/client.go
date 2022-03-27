package client

// Client is a remoteconfig client
// This structure is *not* thread safe
type Client struct {
	products map[string]struct{}
}

func (c *Client) AddProduct(product string) {
	c.products[product] = struct{}{}
}

func (c *Client) RemoveProduct(product string) {
	delete(c.products, product)
}

func (c *Client) GetConfigs(time int64) {

}

type File struct {
	Path string
	Raw  []byte
}

type Update struct {
	Roots       [][]byte
	Targets     []byte
	TargetFiles []File
}

func (c *Client) Update(update Update) error {

	return nil
}
