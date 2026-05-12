package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"smart-money-backend/models"
)

var politicianDateFormats = []string{
	"2006-01-02",
	"01/02/2006",
	"1/2/2006",
	"January 2, 2006",
}

func parsePoliticianDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	for _, layout := range politicianDateFormats {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// FMP house disclosure response shape.
type fmpHouseTrade struct {
	DisclosureDate  string `json:"disclosureDate"`
	TransactionDate string `json:"transactionDate"`
	Representative  string `json:"representative"`
	Ticker          string `json:"ticker"`
	TransactionType string `json:"transactionType"`
	Amount          string `json:"amount"`
	District        string `json:"district"`
}

// FMP senate trading response shape.
type fmpSenateTrade struct {
	DateRecieved    string `json:"dateRecieved"`
	TransactionDate string `json:"transactionDate"`
	Owner           string `json:"owner"`
	Ticker          string `json:"ticker"`
	Amount          string `json:"amount"`
	Type            string `json:"type"`
	Comment         string `json:"comment"`
}

func fmpGET(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SmartMoneyTracker/1.0 research@aiedgehq.co")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[politician] GET %s → status=%d", url, resp.StatusCode)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// FetchPoliticianTrades fetches House + Senate congressional disclosures via FMP API.
func FetchPoliticianTrades(ticker string) ([]models.PoliticianTrade, error) {
	apiKey := os.Getenv("FMP_API_KEY")
	if apiKey == "" {
		log.Printf("[politician] FMP_API_KEY not set — skipping politician trades")
		return []models.PoliticianTrade{}, nil
	}

	upperTicker := strings.ToUpper(strings.TrimSpace(ticker))
	cutoff := time.Now().AddDate(-1, 0, 0)

	var trades []models.PoliticianTrade

	// --- House disclosures ---
	houseURL := fmt.Sprintf(
		"https://financialmodelingprep.com/api/v4/house-disclosure?symbol=%s&apikey=%s",
		upperTicker, apiKey,
	)
	if body, err := fmpGET(houseURL); err == nil {
		var rows []fmpHouseTrade
		if json.Unmarshal(body, &rows) == nil {
			for _, r := range rows {
				dateStr := r.TransactionDate
				if dateStr == "" {
					dateStr = r.DisclosureDate
				}
				d, ok := parsePoliticianDate(dateStr)
				if !ok || d.Before(cutoff) {
					continue
				}
				trades = append(trades, models.PoliticianTrade{
					Name:      shortenName(r.Representative),
					Role:      "House",
					Party:     "",
					Action:    strings.ToUpper(r.TransactionType),
					Amount:    r.Amount,
					FiledDate: dateStr,
				})
			}
		} else {
			log.Printf("[politician] house parse error for %s", upperTicker)
		}
	} else {
		log.Printf("[politician] house fetch error: %v", err)
	}

	// --- Senate trades ---
	senateURL := fmt.Sprintf(
		"https://financialmodelingprep.com/api/v4/senate-trading?symbol=%s&apikey=%s",
		upperTicker, apiKey,
	)
	if body, err := fmpGET(senateURL); err == nil {
		var rows []fmpSenateTrade
		if json.Unmarshal(body, &rows) == nil {
			for _, r := range rows {
				dateStr := r.TransactionDate
				if dateStr == "" {
					dateStr = r.DateRecieved
				}
				d, ok := parsePoliticianDate(dateStr)
				if !ok || d.Before(cutoff) {
					continue
				}
				trades = append(trades, models.PoliticianTrade{
					Name:      shortenName(r.Owner),
					Role:      "Senate",
					Party:     "",
					Action:    strings.ToUpper(r.Type),
					Amount:    r.Amount,
					FiledDate: dateStr,
				})
			}
		} else {
			log.Printf("[politician] senate parse error for %s", upperTicker)
		}
	} else {
		log.Printf("[politician] senate fetch error: %v", err)
	}

	// Sort most-recent-first.
	for i := 1; i < len(trades); i++ {
		for j := i; j > 0; j-- {
			di, _ := parsePoliticianDate(trades[j].FiledDate)
			dj, _ := parsePoliticianDate(trades[j-1].FiledDate)
			if di.After(dj) {
				trades[j], trades[j-1] = trades[j-1], trades[j]
			} else {
				break
			}
		}
	}

	if len(trades) > 10 {
		trades = trades[:10]
	}
	if trades == nil {
		trades = []models.PoliticianTrade{}
	}

	log.Printf("[politician] %s → %d trades (house+senate)", upperTicker, len(trades))
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
