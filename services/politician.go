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

// quiverTrade maps the Quiver Quantitative congressional trading response.
type quiverTrade struct {
	Date           string  `json:"Date"`
	Ticker         string  `json:"Ticker"`
	Representative string  `json:"Representative"`
	Transaction    string  `json:"Transaction"`
	Amount         string  `json:"Amount"`
	House          string  `json:"House"`
	Party          string  `json:"Party"`
	Range          *string `json:"Range"`
}

// FetchPoliticianTrades fetches congressional trade disclosures via Quiver Quantitative.
// Falls back to a graceful empty response if the API key is not configured.
func FetchPoliticianTrades(ticker string) ([]models.PoliticianTrade, error) {
	apiKey := os.Getenv("QUIVER_API_KEY")
	if apiKey == "" {
		log.Printf("[politician] QUIVER_API_KEY not set — skipping politician trades")
		return []models.PoliticianTrade{}, nil
	}

	url := fmt.Sprintf("https://api.quiverquant.com/beta/historical/congresstrading/%s", strings.ToUpper(ticker))

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []models.PoliticianTrade{}, nil
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SmartMoneyTracker/1.0 research@aiedgehq.co")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[politician] quiver request error: %v", err)
		return []models.PoliticianTrade{}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []models.PoliticianTrade{}, nil
	}

	log.Printf("[politician] quiver GET %s → status=%d", url, resp.StatusCode)

	if resp.StatusCode != 200 {
		log.Printf("[politician] quiver error body: %s", snippet(body))
		return []models.PoliticianTrade{}, nil
	}

	var raw []quiverTrade
	if err := json.Unmarshal(body, &raw); err != nil {
		log.Printf("[politician] quiver parse error: %v", err)
		return []models.PoliticianTrade{}, nil
	}

	// Filter to last 12 months and sort most-recent-first.
	cutoff := time.Now().AddDate(-1, 0, 0)

	type dated struct {
		trade models.PoliticianTrade
		date  time.Time
	}
	var found []dated

	for _, t := range raw {
		d, ok := parseHSWDate(t.Date)
		if !ok || d.Before(cutoff) {
			continue
		}

		amount := t.Amount
		if amount == "" && t.Range != nil {
			amount = *t.Range
		}

		chamber := t.House
		if chamber == "" {
			chamber = "Congress"
		}

		found = append(found, dated{
			trade: models.PoliticianTrade{
				Name:      shortenName(t.Representative),
				Role:      chamber,
				Party:     abbreviateParty(t.Party),
				Action:    strings.ToUpper(t.Transaction),
				Amount:    amount,
				FiledDate: t.Date,
			},
			date: d,
		})
	}

	// Insertion sort: most recent first.
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

	if trades == nil {
		trades = []models.PoliticianTrade{}
	}

	log.Printf("[politician] %s → %d trades after filtering", ticker, len(trades))
	return trades, nil
}

func snippet(b []byte) string {
	if len(b) > 150 {
		return string(b[:150])
	}
	return string(b)
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
