package miutil

import (
	"bytes"
	"io"
	"net/http"

	"github.com/goccy/go-json"
)

// Post は指定のurlにJSONをPOSTする
func Post(url string, b []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json") // ないと Unsupported Media Type
	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// Post は指定のurlにJSONをPOSTし、ResponseのJSONを受け取る
func Post2[T any](url string, b io.Reader) (*T, error) {
	req, err := http.NewRequest(http.MethodPost, url, b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json") // ないと Unsupported Media Type
	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	var out T
	err = decodeBody(resp, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Post は指定のurlにJSONをPOSTし、ResponseのJSONを受け取る
func Post3(url string, b io.Reader, out any) error {
	req, err := http.NewRequest(http.MethodPost, url, b)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json") // ないと Unsupported Media Type
	client := new(http.Client)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	err = decodeBody(resp, out)
	if err != nil {
		return err
	}
	return nil
}

func decodeBody(resp *http.Response, out any) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}
