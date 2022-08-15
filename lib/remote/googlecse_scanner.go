package remote

import (
	"context"
	"errors"
	"fmt"
	"github.com/sundowndev/phoneinfoga/v2/lib/number"
	"google.golang.org/api/customsearch/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"os"
	"strconv"
)

const GoogleCSE = "googlecse"

type googleCSEScanner struct {
	Cx         string
	ApiKey     string
	MaxResults int64
}

type ResultItem struct {
	Title string `json:"title,omitempty" console:"Title,omitempty"`
	URL   string `json:"url,omitempty" console:"URL,omitempty"`
}

type GoogleCSEScannerResponse struct {
	Homepage    string       `json:"homepage,omitempty" console:"Homepage,omitempty"`
	ResultCount int          `json:"result_count" console:"Result count"`
	Items       []ResultItem `json:"items,omitempty" console:"Items,omitempty"`
}

func NewGoogleCSEScanner() Scanner {
	// CSE limits you to 10 pages of results with max 10 results per page
	// We only fetch the first page of results by default for each request
	maxResults := 10
	if v := os.Getenv("GOOGLECSE_MAX_RESULTS"); v != "" {
		val, err := strconv.Atoi(v)
		if err == nil {
			if val > 100 {
				val = 100
			}
			maxResults = val
		}
	}

	return &googleCSEScanner{
		Cx:         os.Getenv("GOOGLECSE_CX"),
		ApiKey:     os.Getenv("GOOGLECSE_API_KEY"),
		MaxResults: int64(maxResults),
	}
}

func (s *googleCSEScanner) Name() string {
	return GoogleCSE
}

func (s *googleCSEScanner) ShouldRun(_ number.Number) bool {
	if s.Cx == "" || s.ApiKey == "" {
		return false
	}
	return true
}

func (s *googleCSEScanner) Scan(n number.Number) (interface{}, error) {
	var allItems []*customsearch.Result
	var dorks []*GoogleSearchDork

	dorks = append(dorks, getDisposableProvidersDorks(n)...)
	dorks = append(dorks, getReputationDorks(n)...)
	dorks = append(dorks, getIndividualsDorks(n)...)
	dorks = append(dorks, getGeneralDorks(n)...)

	customsearchService, err := customsearch.NewService(context.Background(), option.WithAPIKey(s.ApiKey))
	if err != nil {
		return nil, err
	}

	for _, req := range dorks {
		items, err := s.search(customsearchService, req.Dork)
		if err != nil {
			if s.isRateLimit(err) {
				return nil, errors.New("rate limit exceeded, see https://developers.google.com/custom-search/v1/overview#pricing")
			}
			return nil, err
		}
		allItems = append(allItems, items...)
	}

	var data GoogleCSEScannerResponse
	data.Homepage = fmt.Sprintf("https://cse.google.com/cse?cx=%s", s.Cx)
	for _, item := range allItems {
		data.Items = append(data.Items, ResultItem{
			Title: item.Title,
			URL:   item.Link,
		})
	}
	data.ResultCount = len(allItems)

	return data, nil
}

func (s *googleCSEScanner) search(service *customsearch.Service, q string) ([]*customsearch.Result, error) {
	var results []*customsearch.Result

	offset := int64(0)
	for offset <= s.MaxResults {
		search := service.Cse.List()
		search.Cx(s.Cx)
		search.Q(q)
		search.Start(offset)
		searchQuery, err := search.Do()
		if err != nil {
			return nil, err
		}
		results = append(results, searchQuery.Items...)
		totalResults, err := strconv.Atoi(searchQuery.SearchInformation.TotalResults)
		if err != nil {
			return nil, err
		}
		if totalResults <= int(s.MaxResults) {
			break
		}
		offset += int64(len(searchQuery.Items))
	}

	return results, nil
}

func (s *googleCSEScanner) isRateLimit(theError error) bool {
	if theError == nil {
		return false
	}
	if _, ok := theError.(*googleapi.Error); !ok {
		return false
	}
	if theError.(*googleapi.Error).Code != 429 {
		return false
	}
	return true
}
