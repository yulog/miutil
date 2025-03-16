package miutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
)

type Client struct {
	Host       string
	Credential string
}

type Request struct {
	url  string
	body io.Reader
}

func NewClient(host, credential string) *Client {
	return &Client{Host: host, Credential: credential}
}

func (c *Client) NewPostRequest(u string, body map[string]any) (*Request, error) {
	body["i"] = c.Credential
	u, err := url.JoinPath(c.Host, u) // https://mattn.kaoriya.net/software/lang/go/20220401001651.htm
	if err != nil {
		return &Request{}, err
	}
	buf := bytes.NewBuffer(nil)
	err = json.NewEncoder(buf).Encode(body)
	if err != nil {
		return &Request{}, err
	}

	return &Request{url: u, body: buf}, nil
}

func (r *Request) Do(out any) error {
	return Post3(r.url, r.body, out)
}
