package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"

	"budgetbridge/ynab"
)

type CategoriesCache struct {
	client   *ynab.Client
	budgetID string
	path     string
	enabled  bool
}

func (c *CategoriesCache) Categories(ctx context.Context) ([]ynab.Category, error) {
	var categories []ynab.Category
	if err := c.get(&categories); err == nil {
		return categories, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	categories, err := c.fetch(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.put(categories); err != nil {
		return nil, err
	}
	return categories, nil
}

func (c *CategoriesCache) put(v interface{}) error {
	f, err := os.Create(c.path)
	if err != nil {
		return err
	}
	defer f.Close()
	encoder := json.NewEncoder(bufio.NewWriter(f))
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func (c *CategoriesCache) get(v interface{}) error {
	f, err := os.Open(c.path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(bufio.NewReader(f)).Decode(v)
}

func (c *CategoriesCache) fetch(ctx context.Context) ([]ynab.Category, error) {
	categoriesResponse, err := c.client.Categories(ctx, ynab.CategoriesRequest{
		BudgetID: c.budgetID,
	})
	if err != nil {
		return nil, err
	}
	var categories []ynab.Category
	for _, group := range categoriesResponse.CategoryGroups {
		categories = append(categories, group.Categories...)
	}
	return categories, nil
}
