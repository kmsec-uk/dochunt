package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var (
	searchUrl = "https://urlscan.io/api/v1/search"
	// default rate limit for urlscan. The retrieval limit for a free urlscan ulser account is 120.
	defaultRateLimit = time.Duration(500 * time.Millisecond)
	// default page size for search results.
	defaultPageSize             = 100
	ErrRateLimitExhausted error = errors.New("client backed off to the maximum rate limit (>30s) but is still getting unexpected status code")
	maxRetries                  = 5
)

// HTTPError is a custom error that holds the HTTP status code
type HTTPError struct {
	StatusCode int
	Err        error
}

func (e *HTTPError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("HTTP %d: %v", e.StatusCode, e.Err)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

func (e *HTTPError) Unwrap() error {
	return e.Err
}

type UrlscanClient struct {
	client    *http.Client
	limiter   *rate.Limiter
	delay     time.Duration
	baseDelay time.Duration
	apikey    string
	pageSize  int

	mu sync.Mutex
}

type Page struct {
	Url string `json:"url"`
}

type Task struct {
	Time       time.Time `json:"time"`
	Visibility string    `json:"visibility"`
}

type UrlscanSearchPage struct {
	Results []UrlscanSearchResult `json:"results"`
	Total   uint64                `json:"total"`
	HasMore bool                  `json:"has_more"`
}

type UrlscanSearchResult struct {
	Task Task   `json:"task"`
	Page Page   `json:"page"`
	Id   string `json:"_id"`
	Sort Sort   `json:"sort"`
}

type Sort struct {
	Timestamp int64
	Id        string
}

func (s *Sort) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if l := len(raw); l != 2 {
		return fmt.Errorf("unexpected length of sort key: want 2, got %d", l)
	}
	if err := json.Unmarshal(raw[0], &s.Timestamp); err != nil {
		return err
	}
	if err := json.Unmarshal(raw[1], &s.Id); err != nil {
		return err
	}
	return nil
}
func (s *Sort) Key() (string, error) {
	if s.Timestamp == 0 || s.Id == "" {
		return "", errors.New("call to sort.Key() with empty sort properties")
	}
	return strconv.Itoa(int(s.Timestamp)) + "," + s.Id, nil
}

// NewUrlscanClient creates a new client for interacting
// with the urlscan api
func NewUrlscanClient(apikey string) *UrlscanClient {
	return &UrlscanClient{
		client:    &http.Client{},
		limiter:   rate.NewLimiter(rate.Every(defaultRateLimit), 1),
		delay:     defaultRateLimit,
		baseDelay: defaultRateLimit,
		pageSize:  defaultPageSize,
	}
}

// WithRateLimit defines a rate limit for the client's http
// requests.
// The sensible default here is 500ms, as this is what a regular
// urlscan user is given.
// TODO: func to configure smart quota  via https://urlscan.io/api/v1/quotas
func (u *UrlscanClient) WithRateLimit(d time.Duration) *UrlscanClient {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.baseDelay = d
	u.delay = d
	u.limiter = rate.NewLimiter(rate.Every(d), 1)
	return u
}

// WithPageSize defines the page size for the search api endpoint.
// By defaut you get 100 results per page, but more (or less) can
// be configured within the bounds of your account's quota.
func (u *UrlscanClient) WithPageSize(i int) *UrlscanClient {
	u.pageSize = i
	return u
}

// recoverRate ramps down the
// request interval by cutting the rate limit
// in half until we're back to baseline
func (u *UrlscanClient) recoverRate() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.delay == u.baseDelay {
		return
	}
	u.delay /= 2
	if u.delay < u.baseDelay {
		u.delay = u.baseDelay
	}
	u.limiter.SetLimit(rate.Every(u.delay))
}

// backoff ramps up request interval to ease
// off pressure on CommonCrawl
func (u *UrlscanClient) backoff() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.delay > 30*time.Second {
		return ErrRateLimitExhausted
	}
	u.delay *= 2
	u.limiter.SetLimit(rate.Every(u.delay))
	return nil
}

// Result encapsulates urlscan result items and
// errors for sending via channel
type Result struct {
	Item  UrlscanSearchResult
	Error error
}

// Search searches urlscan with a given query.
// You can optionally pass a `since` unix timestamp
// (seconds or ms) in string format, which formats the query
// accordingly: `date:>{since} AND ({query})`.
// This function automatically pages through results.
func (u *UrlscanClient) Search(ctx context.Context, query, since string) (chan Result, error) {
	if since != "" {
		query = fmt.Sprintf("date:>%s AND (%s)", since, query)
	}

	ch := make(chan Result, 100)
	currentPage := 1
	go func() {
		defer close(ch)
		var sortKey string
		for {
			req, err := http.NewRequestWithContext(ctx, "GET", searchUrl, nil)
			if err != nil {
				ch <- Result{Error: fmt.Errorf("making request: %w", err)}
				return
			}
			v := req.URL.Query()
			v.Add("size", strconv.Itoa(u.pageSize))
			v.Add("q", query)
			// sort key only added to params if it has been set by
			// previous iteration
			if sortKey != "" {
				v.Add("search_after", sortKey)
				currentPage += 1
			}
			req.URL.RawQuery = v.Encode()
			log.Printf("urlscan search: getting page %d: %s", currentPage, req.URL)

			req.Header.Add("api-key", u.apikey)
			res, err := u.doReq(req)
			if err != nil {
				ch <- Result{Error: fmt.Errorf("doing request: %w", err)}
				return
			}
			defer res.Body.Close()
			var page UrlscanSearchPage
			if err := json.NewDecoder(res.Body).Decode(&page); err != nil {
				ch <- Result{Error: fmt.Errorf("decoding page json: %w", err)}
				return
			}
			var count int
			for _, item := range page.Results {
				count += 1
				ch <- Result{Item: item}
				sk, err := item.Sort.Key()
				if err != nil {
					ch <- Result{Error: fmt.Errorf("generating sort key: %w", err)}
					return
				}
				sortKey = sk
			}
			// if we have fewer than our desired client's page size,
			// there are no more results.
			if (count < u.pageSize) || (count == 0) {
				return
			}
		}
	}()
	return ch, nil
}

// GetDom returns the raw http.Body (io.ReadCloser)
// that is sufficient to be parsed by a html parser
// i.e. the GDoc HTML parser.
func (u *UrlscanClient) GetDom(ctx context.Context, id string) (io.ReadCloser, error) {
	url := "https://urlscan.io/dom/" + id + "/"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	req.Header.Add("api-key", u.apikey)
	res, err := u.doReq(req)
	if err != nil {
		return nil, fmt.Errorf("doing request: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}
	return res.Body, nil
}

// doReq wraps a http request in the responsive rate limiter.
// it throws an error for a non-status 200
func (u *UrlscanClient) doReq(req *http.Request) (*http.Response, error) {
	var lastErr error
	for range maxRetries {
		if err := u.limiter.Wait(req.Context()); err != nil {
			return nil, fmt.Errorf("rate limiter aborted: %w", err)
		}
		res, err := u.client.Do(req)
		if err != nil {
			err := u.backoff()
			if err != nil {
				return nil, fmt.Errorf("backing off: %w: status code %d", err, res.StatusCode)
			}
			lastErr = err
			continue
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			u.recoverRate()
			return res, nil
		}
		if res.StatusCode == http.StatusNotFound {
			return res, errors.New("not found")
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()

			err := u.backoff()
			if err != nil {
				return nil, fmt.Errorf("backing off: %w: status code %d", err, res.StatusCode)
			}

			lastErr = &HTTPError{
				StatusCode: res.StatusCode,
				Err:        fmt.Errorf("unexpected status: %s", res.Status),
			}
			continue
		}
		// unexpected status received, therefore we just exit
		res.Body.Close()
		return nil, &HTTPError{
			StatusCode: res.StatusCode,
			Err:        fmt.Errorf("unexpected status: %s", res.Status),
		}

	}
	return nil, fmt.Errorf("max retries exhausted. Last error: %w", lastErr)

}
