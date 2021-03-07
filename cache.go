package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"budgetbridge/ynab"

	"github.com/rs/zerolog/log"
)

type CachingClient struct {
	client *ynab.Client
	cache  Cache
}

type Cache interface {
	Open() error
	Close() error
	Get(key string, res interface{}) error
	Set(key string, res interface{}) error
}

func (c *CachingClient) CreateTransactions(ctx context.Context, budgetID string, req ynab.CreateTransactionsRequest) (ynab.TransactionsResponse, error) {
	return c.client.CreateTransactions(ctx, budgetID, req)
}

func (c *CachingClient) Transactions(ctx context.Context, req ynab.TransactionsRequest) (ynab.TransactionsResponse, error) {
	return c.client.Transactions(ctx, req)
}

func (c *CachingClient) Budgets(ctx context.Context) (ynab.BudgetsResponse, error) {
	cacheKey := "budgets"

	var res ynab.BudgetsResponse
	err := c.cache.Get(cacheKey, &res)
	if errors.Is(err, errNotFound) {
		res, err := c.client.Budgets(ctx)
		if err != nil {
			return res, fmt.Errorf("failed to fetch: %s", err)
		}
		if err := c.cache.Set(cacheKey, &res); err != nil {
			return res, fmt.Errorf("failed to write to cache: %s", err)
		}
		return res, err
	}
	if err != nil {
		// Some non-recoverable error.
		return res, err
	}
	return res, nil
}

func (c *CachingClient) Categories(ctx context.Context, req ynab.CategoriesRequest) (ynab.CategoriesResponse, error) {
	cacheKey := fmt.Sprintf("categories/%s", req.BudgetID)

	var res ynab.CategoriesResponse
	err := c.cache.Get(cacheKey, &res)
	if errors.Is(err, errNotFound) {
		res, err := c.client.Categories(ctx, req)
		if err != nil {
			return res, fmt.Errorf("failed to fetch: %s", err)
		}
		if err := c.cache.Set(cacheKey, &res); err != nil {
			return res, fmt.Errorf("failed to write to cache: %s", err)
		}
		return res, err
	}
	if err != nil {
		// Some non-recoverable error.
		return res, err
	}
	return res, nil
}

var errNotFound error = errors.New("not found")

type FileCache struct {
	path          string
	createMissing bool
	cache         map[string]json.RawMessage
}

func (c *FileCache) Open() error {
	cache, err := c.loadFromDisk()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err != nil && c.createMissing {
		log.Debug().Msg("init cache directory")
		dir := filepath.Dir(c.path)
		c.cache = make(map[string]json.RawMessage)
		return os.MkdirAll(dir, os.ModePerm)
	}
	c.cache = cache
	return nil
}

func (c *FileCache) Get(key string, res interface{}) error {
	value, ok := c.cache[key]
	if !ok {
		log.Debug().Str("key", key).Msg("cache miss")
		return errNotFound
	}
	if err := json.Unmarshal(value, res); err != nil {
		return err
	}
	log.Debug().Str("key", key).Msg("cache hit")
	return nil
}

func (c *FileCache) Set(key string, res interface{}) error {
	var value json.RawMessage
	value, err := json.Marshal(res)
	if err != nil {
		return err
	}
	c.cache[key] = value
	return nil
}

func (c *FileCache) Close() error {
	f, err := os.Create(c.path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(c.cache)
}

func (c *FileCache) loadFromDisk() (map[string]json.RawMessage, error) {
	f, err := os.Open(c.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var fullCache map[string]json.RawMessage
	if err := json.NewDecoder(bufio.NewReader(f)).Decode(&fullCache); err != nil {
		return nil, err
	}
	return fullCache, nil
}
