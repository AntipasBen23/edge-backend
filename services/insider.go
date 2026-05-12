package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"smart-money-backend/models"
)

type edgarHit struct {
	Source struct {
		EntityName   string `json:"entity_name"`
		FileDate     string `json:"file_date"`
		FormType     string `json:"form_type"`
		DisplayNames []struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"display_names"`
	} `json:"_source"`
}

type edgarSearchResult struct {
	Hits struct {
		Hits []edgarHit `json:"hits"`
	} `json:"hits"`
}

// FetchInsiderTrades queries SEC EDGAR full-text search for Form 4 filings
// mentioning the given ticker over the last 90 days.
func FetchInsiderTrades(ticker string) ([]models.InsiderTrade, error) {
	endDate := time.Now().Format("2006-01-02")
	// 90-day window: insider filings can be delayed and sparse over 30 days
	startDate := time.Now().AddDate(0, -3, 0).Format("2006-01-02")

	url := fmt.Sprintf(
		"https://efts.sec.gov/LATEST/search-index?q=%%22%s%%22&forms=4&dateRange=custom&startdt=%s&enddt=%s",
		strings.ToUpper(ticker), startDate, endDate,
	)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating insider trades request: %w", err)
	}
	// SEC EDGAR requires a User-Agent identifying the requester.
	req.Header.Set("User-Agent", "SmartMoneyTracker/1.0 research@aiedgehq.co")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching insider trades: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading insider trades body: %w", err)
	}

	var result edgarSearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		// EDGAR sometimes returns HTML error pages — treat as no data rather than hard fail.
		return []models.InsiderTrade{}, nil
	}

	var trades []models.InsiderTrade
	seen := make(map[string]bool)

	for _, hit := range result.Hits.Hits {
		name := hit.Source.EntityName
		role := "Insider"

		if len(hit.Source.DisplayNames) > 0 {
			name = hit.Source.DisplayNames[0].Name
			if hit.Source.DisplayNames[0].Role != "" {
				role = hit.Source.DisplayNames[0].Role
			}
		}

		key := name + hit.Source.FileDate
		if seen[key] {
			continue
		}
		seen[key] = true

		action := "FORM 4 FILED"
		if hit.Source.FormType == "4/A" {
			action = "FORM 4/A AMENDED"
		}

		trades = append(trades, models.InsiderTrade{
			Name:      name,
			Role:      role,
			Action:    action,
			Amount:    "See SEC filing",
			FiledDate: hit.Source.FileDate,
		})

		if len(trades) >= 10 {
			break
		}
	}

	return trades, nil
}
