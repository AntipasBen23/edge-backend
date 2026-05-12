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

type hswTransaction struct {
	Representative   string `json:"representative"`
	Party            string `json:"party"`
	TransactionDate  string `json:"transaction_date"`
	Ticker           string `json:"ticker"`
	Type             string `json:"type"`
	Amount           string `json:"amount"`
	AssetDescription string `json:"asset_description"`
}

// FetchPoliticianTrades pulls all congressional disclosures from House Stock Watcher
// and returns trades for the given ticker filed in the last 30 days.
func FetchPoliticianTrades(ticker string) ([]models.PoliticianTrade, error) {
	const url = "https://house-stock-watcher-data.s3-us-west-2.amazonaws.com/data/all_transactions.json"

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching politician trades: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading politician trades body: %w", err)
	}

	var transactions []hswTransaction
	if err := json.Unmarshal(body, &transactions); err != nil {
		return nil, fmt.Errorf("parsing politician trades JSON: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -30)
	upperTicker := strings.ToUpper(ticker)

	var trades []models.PoliticianTrade
	for _, t := range transactions {
		if strings.ToUpper(t.Ticker) != upperTicker {
			continue
		}

		tradeDate, err := time.Parse("2006-01-02", t.TransactionDate)
		if err != nil || tradeDate.Before(cutoff) {
			continue
		}

		trades = append(trades, models.PoliticianTrade{
			Name:      shortenName(t.Representative),
			Role:      fmt.Sprintf("House (%s)", abbreviateParty(t.Party)),
			Party:     abbreviateParty(t.Party),
			Action:    strings.ToUpper(t.Type),
			Amount:    t.Amount,
			FiledDate: t.TransactionDate,
		})
	}

	if len(trades) > 10 {
		trades = trades[:10]
	}

	return trades, nil
}

func shortenName(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) < 2 {
		return name
	}
	return string([]rune(parts[0])[:1]) + ". " + parts[len(parts)-1]
}

func abbreviateParty(party string) string {
	switch strings.ToLower(party) {
	case "democrat", "democratic":
		return "D"
	case "republican":
		return "R"
	case "independent":
		return "I"
	default:
		if len(party) > 0 {
			return strings.ToUpper(string([]rune(party)[:1]))
		}
		return party
	}
}
