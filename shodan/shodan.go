package shodan

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-querystring/query"
)

const (
	baseURL        = "https://api.shodan.io"
	exploitBaseURL = "https://exploits.shodan.io/api"
	streamBaseURL  = "https://stream.shodan.io"
)

func getErrorFromResponse(r *http.Response) error {
	errorResponse := new(struct {
		Error string `json:"error"`
	})
	message, err := ioutil.ReadAll(r.Body)
	if err == nil {
		if err := json.Unmarshal(message, errorResponse); err == nil {
			return errors.New(errorResponse.Error)
		} else {
			return errors.New(strings.TrimSpace(string(message)))
		}
	}

	return ErrBodyRead
}

type Client struct {
	Token          string
	BaseURL        string
	ExploitBaseURL string
	StreamBaseURL  string
	StreamChan     chan HostData

	Client *http.Client
}

func NewClient(client *http.Client, token string) *Client {
	if client == nil {
		client = http.DefaultClient
	}

	return &Client{
		Token:          token,
		BaseURL:        baseURL,
		ExploitBaseURL: exploitBaseURL,
		StreamBaseURL:  streamBaseURL,
		StreamChan:     make(chan HostData),
		Client:         client,
	}
}

func (c *Client) buildURL(base, path string, params interface{}) (string, error) {
	baseURL, err := url.Parse(base + path)
	if err != nil {
		return "", err
	}

	qs, err := query.Values(params)
	if err != nil {
		return baseURL.String(), err
	}

	qs.Add("key", c.Token)

	baseURL.RawQuery = qs.Encode()

	return baseURL.String(), nil
}

func (c *Client) buildBaseURL(path string, params interface{}) (string, error) {
	return c.buildURL(c.BaseURL, path, params)
}

func (c *Client) buildExploitBaseURL(path string, params interface{}) (string, error) {
	return c.buildURL(c.ExploitBaseURL, path, params)
}

func (c *Client) buildStreamBaseURL(path string, params interface{}) (string, error) {
	return c.buildURL(c.StreamBaseURL, path, params)
}

func (c *Client) sendRequest(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	}

	res, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, getErrorFromResponse(res)
	}

	return res, nil
}

func (c *Client) parseResponse(destination interface{}, body io.Reader) error {
	var err error

	if w, ok := destination.(io.Writer); ok {
		_, err = io.Copy(w, body)
	} else {
		decoder := json.NewDecoder(body)
		err = decoder.Decode(destination)
	}

	return err
}

func (c *Client) executeRequest(method, path string, destination interface{}, body io.Reader) error {
	res, err := c.sendRequest(method, path, body)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	return c.parseResponse(destination, res.Body)
}

func (c *Client) executeStreamRequest(method, path string, ch chan []byte) error {
	res, err := c.sendRequest(method, path, nil)
	if err != nil {
		return err
	}

	go func() {
		reader := bufio.NewReader(res.Body)

		for {
			chunk, err := reader.ReadBytes('\n')
			if err != nil {
				res.Body.Close()
				close(ch)
				break
			}

			ch <- chunk
		}
	}()

	return nil
}
