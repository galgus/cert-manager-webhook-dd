package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/schema"
)

// DefaultTimeout api requests after 180s
const DefaultTimeout = 180 * time.Second

// Endpoints
const Endpoint = "https://simple-api.dondominio.net"

var encoder = schema.NewEncoder()

// Client represents a client to call the DD API
type Client struct {
	// AppKey holds the Application key
	AppKey string

	// AppSecret holds the Application secret key
	AppSecret string

	// API endpoint
	endpoint string

	// Client is the underlying HTTP client used to run the requests. It may be overloaded but a default one is instanciated in ``NewClient`` by default.
	Client *http.Client

	// Logger is used to log HTTP requests and responses.
	Logger Logger

	// Ensures that the timeDelta function is only ran once
	// sync.Once would consider init done, even in case of error
	// hence a good old flag
	timeDelta atomic.Value

	// Timeout configures the maximum duration to wait for an API requests to complete
	Timeout time.Duration

	// UserAgent configures the user-agent indication that will be sent in the requests to DDcloud API
	UserAgent string
}

// NewClient represents a new client to call the API
func NewClient(endpoint, appKey, appSecret string) (*Client, error) {
    var httpClient http.Client;
    if proxyUrl, err := url.Parse(os.Getenv("PROXY")); err == nil {
        fmt.Printf("PROXY: %s\n", proxyUrl)
        httpClient = http.Client{
            Transport: &http.Transport{
                Proxy: http.ProxyURL(proxyUrl),
            },
        }
    } else {
        httpClient = http.Client{}
    }
	client := Client{
		AppKey:    appKey,
		AppSecret: appSecret,
        Client:    &httpClient,
		Timeout:   time.Duration(DefaultTimeout),
	}

	// Get and check the configuration
	if err := client.loadConfig(endpoint); err != nil {
		return nil, err
	}
	return &client, nil
}

// NewEndpointClient will create an API client for specified
// endpoint and load all credentials from environment or
// configuration files
func NewEndpointClient(endpoint string) (*Client, error) {
	return NewClient(endpoint, "", "")
}

// NewDefaultClient will load all it's parameter from environment
// or configuration files
func NewDefaultClient() (*Client, error) {
	return NewClient("", "", "")
}

//
// High level helpers
//

// Ping performs a ping to DD API.
// In fact, ping is just a /auth/time call, in order to check if API is up.
func (c *Client) Ping() error {
	_, err := c.getTime()
	return err
}

// TimeDelta represents the delay between the machine that runs the code and the
// DD API. The delay shouldn't change, let's do it only once.
func (c *Client) TimeDelta() (time.Duration, error) {
	return c.getTimeDelta()
}

// Time returns time from the DD API, by asking GET /auth/time.
func (c *Client) Time() (*time.Time, error) {
	return c.getTime()
}

//
// Common request wrappers
//

// Get is a wrapper for the GET method
func (c *Client) Get(url string, resType interface{}) error {
	return c.CallAPI("GET", url, nil, resType)
}

// Post is a wrapper for the POST method
func (c *Client) Post(url string, reqBody, resType interface{}) error {
	return c.CallAPI("POST", url, reqBody, resType)
}

// Put is a wrapper for the PUT method
func (c *Client) Put(url string, reqBody, resType interface{}) error {
	return c.CallAPI("PUT", url, reqBody, resType)
}

// Delete is a wrapper for the DELETE method
func (c *Client) Delete(url string, resType interface{}) error {
	return c.CallAPI("DELETE", url, nil, resType)
}

// GetWithContext is a wrapper for the GET method
func (c *Client) GetWithContext(ctx context.Context, url string, resType interface{}) error {
	return c.CallAPIWithContext(ctx, "GET", url, nil, resType)
}

// PostWithContext is a wrapper for the POST method
func (c *Client) PostWithContext(ctx context.Context, url string, reqBody, resType interface{}) error {
	return c.CallAPIWithContext(ctx, "POST", url, reqBody, resType)
}

// PutWithContext is a wrapper for the PUT method
func (c *Client) PutWithContext(ctx context.Context, url string, reqBody, resType interface{}) error {
	return c.CallAPIWithContext(ctx, "PUT", url, reqBody, resType)
}

// DeleteWithContext is a wrapper for the DELETE method
func (c *Client) DeleteWithContext(ctx context.Context, url string, resType interface{}) error {
	return c.CallAPIWithContext(ctx, "DELETE", url, nil, resType)
}

// timeDelta returns the time delta between the host and the remote API
func (c *Client) getTimeDelta() (time.Duration, error) {
	d, ok := c.timeDelta.Load().(time.Duration)
	if ok {
		return d, nil
	}

	ddTime, err := c.getTime()
	if err != nil {
		return 0, err
	}

	d = time.Since(*ddTime)
	c.timeDelta.Store(d)

	return d, nil
}

// getTime t returns time from for a given api client endpoint
func (c *Client) getTime() (*time.Time, error) {
	var timestamp int64

	err := c.Get("/auth/time", &timestamp)
	if err != nil {
		return nil, err
	}

	serverTime := time.Unix(timestamp, 0)
	return &serverTime, nil
}

// NewRequest returns a new HTTP request
func (c *Client) NewRequest(method, path string, reqBody interface{}) (*http.Request, error) {
	var err error
	var body = url.Values{}

	if reqBody != nil {
		err = encoder.Encode(reqBody, body)
		if err != nil {
			return nil, err
		}
	}

	body.Set("apiuser", c.AppKey)
	body.Set("apipasswd", c.AppSecret)

	//fmt.Fprintf(os.Stdout, "url: %s, body %s\n", path, body.Encode())

	target := fmt.Sprintf("%s%s", c.endpoint, path)
	req, err := http.NewRequest(method, target, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}

	// Inject headers
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")
	req.Header.Add("Accept", "application/json")

	// Send the request with requested timeout
	c.Client.Timeout = c.Timeout

	if c.UserAgent != "" {
		req.Header.Set("User-Agent", "github.com/galgus/go-dd ("+c.UserAgent+")")
	} else {
		req.Header.Set("User-Agent", "github.com/galgus/go-dd")
	}

	return req, nil
}

// Do sends an HTTP request and returns an HTTP response
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c.Logger != nil {
		c.Logger.LogRequest(req)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if c.Logger != nil {
		c.Logger.LogResponse(resp)
	}
	return resp, nil
}

// CallAPI is the lowest level call helper. If needAuth is true,
// inject authentication headers and sign the request.
//
// Request signature is a sha1 hash on following fields, joined by '+':
// - applicationSecret (from Client instance)
// - consumerKey (from Client instance)
// - capitalized method (from arguments)
// - full request url, including any query string argument
// - full serialized request body
// - server current time (takes time delta into account)
//
// Call will automatically assemble the target url from the endpoint
// configured in the client instance and the path argument. If the reqBody
// argument is not nil, it will also serialize it as json and inject
// the required Content-Type header.
//
// If everything went fine, unmarshall response into resType and return nil
// otherwise, return the error
func (c *Client) CallAPI(method, path string, reqBody, resType interface{}) error {
	return c.CallAPIWithContext(context.Background(), method, path, reqBody, resType)
}

// CallAPIWithContext is the lowest level call helper. If needAuth is true,
// inject authentication headers and sign the request.
//
// Request signature is a sha1 hash on following fields, joined by '+':
// - applicationSecret (from Client instance)
// - consumerKey (from Client instance)
// - capitalized method (from arguments)
// - full request url, including any query string argument
// - full serialized request body
// - server current time (takes time delta into account)
//
// # Context is used by http.Client to handle context cancelation
//
// Call will automatically assemble the target url from the endpoint
// configured in the client instance and the path argument. If the reqBody
// argument is not nil, it will also serialize it as json and inject
// the required Content-Type header.
//
// If everything went fine, unmarshall response into resType and return nil
// otherwise, return the error
func (c *Client) CallAPIWithContext(ctx context.Context, method, path string, reqBody, resType interface{}) error {
	req, err := c.NewRequest(method, path, reqBody)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	response, err := c.Do(req)
	if err != nil {
		return err
	}
	return c.UnmarshalResponse(response, resType)
}

// UnmarshalResponse checks the response and unmarshals it into the response
// type if needed Helper function, called from CallAPI
func (c *Client) UnmarshalResponse(response *http.Response, resType interface{}) error {
	// Read all the response body
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	// < 200 && >= 300 : API error
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		apiError := &APIError{Code: response.StatusCode}
		if err = json.Unmarshal(body, apiError); err != nil {
			apiError.Message = string(body)
		}
		apiError.QueryID = response.Header.Get("X-Dd-QueryID")

		return apiError
	}

	// Nothing to unmarshal
	if len(body) == 0 || resType == nil {
		return nil
	}

	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	return d.Decode(&resType)
}
