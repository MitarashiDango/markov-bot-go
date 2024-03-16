package blog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/paralleltree/markov-bot-go/lib"
)

type MastodonStatusVisibility string

const (
	MastodonStatusPublic   = "public"
	MastodonStatusUnlisted = "unlisted"
	MastodonStatusPrivate  = "private"
	MastodonStatusDirect   = "direct"
)

type MastodonClient struct {
	Domain         string
	AccessToken    string
	PostVisibility string
	client         *http.Client
}

func NewMastodonClient(domain, accessToken string, postVisibility string) BlogClient {
	return &MastodonClient{
		Domain:         domain,
		AccessToken:    accessToken,
		PostVisibility: MastodonStatusUnlisted,
		client:         &http.Client{},
	}
}

func (c *MastodonClient) GetPostsFetcher() lib.ChunkIteratorFunc[string] {
	userId := ""
	maxId := ""
	return func() ([]string, bool, error) {
		if userId == "" {
			gotUserId, err := c.FetchUserId()
			if err != nil {
				return nil, false, fmt.Errorf("fetch user id: %w", err)
			}
			userId = gotUserId
		}

		chunkSize := 100
		statuses, hasNext, nextMaxId, err := c.fetchPublicStatusesChunk(userId, chunkSize, maxId)
		if err != nil {
			return nil, false, err
		}
		maxId = nextMaxId
		return statuses, hasNext, nil
	}
}

// Returns status slice and minimum status id to fetch next older statuses.
// This function may returns statuses lesser than specified count because this exlcludes private and direct visibility statuses.
func (c *MastodonClient) fetchPublicStatusesChunk(userId string, count int, maxId string) ([]string, bool, string, error) {
	url := c.buildUrl(fmt.Sprintf("/api/v1/accounts/%s/statuses?limit=%d&exclude_reblogs=1&exclude_replies=1", userId, count))
	if maxId != "" {
		url = fmt.Sprintf("%s&max_id=%s", url, maxId)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, false, "", err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.AccessToken))
	res, err := c.client.Do(req)
	if err != nil {
		return nil, false, "", fmt.Errorf("get statuses: %w", err)
	}
	defer res.Body.Close()
	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, false, "", err
	}

	statuses := []struct {
		Id         string `json:"id"`
		Content    string `json:"content"`
		Visibility string `json:"visibility"`
	}{}
	if err := json.Unmarshal(bytes, &statuses); err != nil {
		return nil, false, "", fmt.Errorf("unmarshal response: %w(%s)", err, bytes)
	}

	if len(statuses) == 0 {
		return nil, false, "", nil
	}

	tagPattern := regexp.MustCompile(`<[^>]*?>`)
	result := make([]string, 0, len(statuses))
	for _, v := range statuses {
		if v.Visibility == MastodonStatusPrivate || v.Visibility == MastodonStatusDirect {
			continue
		}
		// remove tags
		result = append(result, tagPattern.ReplaceAllLiteralString(v.Content, ""))
	}
	return result, true, statuses[len(statuses)-1].Id, nil
}

func (c *MastodonClient) FetchUserId() (string, error) {
	req, err := http.NewRequest("GET", c.buildUrl("/api/v1/accounts/verify_credentials"), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.AccessToken))
	res, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("get account details: %w", err)
	}
	defer res.Body.Close()
	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return "", nil
	}

	account := &struct {
		Id       string `json:"id"`
		UserName string `json:"username"`
	}{}
	if err := json.Unmarshal(bytes, account); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	return account.Id, nil
}

// Posts toot and returns created status id.
func (c *MastodonClient) CreatePost(payload string) error {
	form := url.Values{}
	form.Add("status", payload)
	form.Add("visibility", c.PostVisibility)
	body := strings.NewReader(form.Encode())

	req, err := http.NewRequest("POST", c.buildUrl("/api/v1/statuses"), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.AccessToken))
	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("post status: %w", err)
	}
	defer res.Body.Close()
	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	status := &struct {
		Id string `json:"id"`
	}{}
	if err := json.Unmarshal(bytes, status); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

func (c *MastodonClient) buildUrl(path string) string {
	return fmt.Sprintf("https://%s%s", c.Domain, path)
}
