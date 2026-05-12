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

// HSW uses several date formats depending on the data source.
var hswDateFormats = []string{
	"2006-01-02",
	"01/02/2006",
	"1/2/2006",
	"2006-01-02T15:04:05Z",
	"January 2, 2006",
}

func parseHSWDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	for _, layout := range hswDateFormats {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// FetchPoliticianTrades pulls congressional disclosures from House Stock Watcher
// and returns the 10 most recent trades for the given ticker within the last 12 months.
// We use 12 months because Congress has a 45-day disclosure window, so recent trades
// may appear with transaction dates from several months ago.
func FetchPoliticianTrades(ticker string) ([]models.PoliticianTrade, error) {
	const url = "https://house-stock-watcher-data.s3-us-west-2.amazonaws.com/data/all_transactions.json"

	client := &http.Client{Timeout: 60 * time.Second}
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

	// 12-month window — Congress has 45-day disclosure delay so recent trades
	// often show transaction dates from months prior.
	cutoff := time.Now().AddDate(-1, 0, 0)
	upperTicker := strings.ToUpper(ticker)

	type dated struct {
		trade models.PoliticianTrade
		date  time.Time
	}
	var found []dated

	for _, t := range transactions {
		if strings.ToUpper(strings.TrimSpace(t.Ticker)) != upperTicker {
			continue
		}

		tradeDate, ok := parseHSWDate(t.TransactionDate)
		if !ok || tradeDate.Before(cutoff) {
			continue
		}

		found = append(found, dated{
			trade: models.PoliticianTrade{
				Name:      shortenName(t.Representative),
				Role:      fmt.Sprintf("House (%s)", abbreviateParty(t.Party)),
				Party:     abbreviateParty(t.Party),
				Action:    strings.ToUpper(t.Type),
				Amount:    t.Amount,
				FiledDate: t.TransactionDate,
			},
			date: tradeDate,
		})
	}

	// Sort most recent first (simple insertion — list is small after filtering).
	for i := 1; i < len(found); i++ {
		for j := i; j > 0 && found[j].date.After(found[j-1].date); j-- {
			found[j], found[j-1] = found[j-1], found[j]
		}
	}

	var trades []models.PoliticianTrade
	for i, d := range found {
		if i >= 10 {
			break
		}
		trades = append(trades, d.trade)
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
	switch strings.ToLower(strings.TrimSpace(party)) {
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
