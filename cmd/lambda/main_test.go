package main

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/paralleltree/markov-bot-go/config"
	"github.com/paralleltree/markov-bot-go/handler"
	"github.com/paralleltree/markov-bot-go/lib"
)

func TestRun_WhenModelNotExists_CreatesModel(t *testing.T) {
	// arrange
	inputText := "アルミ缶の上にあるミカン"
	postClient := NewRecordableBlogClient(nil)
	conf := &config.BotConfig{
		FetchClient: NewRecordableBlogClient([]string{inputText}),
		PostClient:  postClient,
		ChainConfig: config.DefaultChainConfig(),
	}
	store := NewMemoryStore()

	// act
	if err := run(conf, store); err != nil {
		t.Errorf("run() should not return error, but got: %v", err)
	}

	// assert
	wantResult := []string{inputText}
	if !reflect.DeepEqual(wantResult, postClient.PostedContents) {
		t.Errorf("unexpected output: want %s, but got %s", inputText, postClient.PostedContents[0])
	}
}

func TestRun_WhenModelIsEmpty_ReturnsGenerateFailedError(t *testing.T) {
	// arrange
	postClient := NewRecordableBlogClient(nil)
	conf := &config.BotConfig{
		FetchClient: NewRecordableBlogClient(nil),
		PostClient:  postClient,
		ChainConfig: config.DefaultChainConfig(),
	}
	store := NewMemoryStore()

	// act
	err := run(conf, store)

	// assert
	if err == nil {
		t.Errorf("run() should return error, but got nil")
	}
	if !errors.Is(err, handler.ErrGenerationFailed) {
		t.Errorf("run() should return ErrGenerateFailed, but got: %v", err)
	}
}

func TestRun_WhenModelAlreadyExistsAndBuildingModelFails_PostsWithExistingModelAndReturnsNoError(t *testing.T) {
	// arrange
	inputText := "アルミ缶の上にあるミカン"
	postClient := NewRecordableBlogClient(nil)
	conf := &config.BotConfig{
		FetchClient: NewRecordableBlogClient([]string{inputText}),
		PostClient:  NewRecordableBlogClient(nil), // discard posted content
		ChainConfig: config.DefaultChainConfig(),
	}
	store := NewMemoryStore()

	// build model
	if err := run(conf, store); err != nil {
		t.Errorf("run() should not return error, but got: %v", err)
	}

	conf = &config.BotConfig{
		FetchClient: &errorBlogClient{},
		PostClient:  postClient,
		ChainConfig: config.ChainConfig{
			FetchStatusCount: 1,
			ExpiresIn:        0, // force building chain
		},
	}

	// act
	if err := run(conf, store); err != nil {
		t.Errorf("run() should not return error, but got: %v", err)
	}

	// assert
	wantResult := []string{inputText}
	if !reflect.DeepEqual(wantResult, postClient.PostedContents) {
		t.Errorf("unexpected output: want %s, but got %s", inputText, postClient.PostedContents[0])
	}
}

type recordableBlogClient struct {
	contents        []string
	contentsFetched bool
	PostedContents  []string
}

func NewRecordableBlogClient(contents []string) *recordableBlogClient {
	return &recordableBlogClient{
		contents: contents,
	}
}

func (f *recordableBlogClient) GetPostsFetcher() lib.ChunkIteratorFunc[string] {
	return func() ([]string, bool, error) {
		if f.contentsFetched {
			return nil, false, nil
		}
		f.contentsFetched = true
		return f.contents, false, nil
	}
}

func (f *recordableBlogClient) CreatePost(body string) error {
	f.PostedContents = append(f.PostedContents, body)
	return nil
}

type errorBlogClient struct{}

func (e *errorBlogClient) GetPostsFetcher() lib.ChunkIteratorFunc[string] {
	return func() ([]string, bool, error) {
		return nil, false, fmt.Errorf("failed to fetch posts")
	}
}

func (e *errorBlogClient) CreatePost(body string) error {
	return fmt.Errorf("failed to create post")
}

type memoryStore struct {
	content []byte
	modTime time.Time
}

func NewMemoryStore() *memoryStore {
	return &memoryStore{
		content: []byte{},
		modTime: time.Now(),
	}
}

func (m *memoryStore) Load() ([]byte, error) {
	return m.content, nil
}

func (m *memoryStore) ModTime() (time.Time, bool, error) {
	return m.modTime, len(m.content) > 0, nil
}

func (m *memoryStore) Save(data []byte) error {
	m.content = data
	return nil
}
