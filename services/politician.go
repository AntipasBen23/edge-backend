package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"smart-money-backend/models"
)

type hswTransaction struct {
	Representative  string `json:"representative"`
	Party           string `json:"party"`
	TransactionDate string `json:"transaction_date"`
	Ticker          string `json:"ticker"`
	Type            string `json:"type"`
	Amount          string `json:"amount"`
}

// Senate Stock Watcher has a different JSON shape.
type sswTransaction struct {
	Senator         string `json:"senator"`
	Party           string `json:"party"`
	TransactionDate string `json:"transaction_date"`
	Ticker          string `json:"ticker"`
	Type            string `json:"type"`
	Amount          string `json:"amount"`
}

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

func doGET(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SmartMoneyTracker/1.0; research@aiedgehq.co)")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("[politician] GET %s → status=%d body_start=%q", url, resp.StatusCode, snippet(body))

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	if len(body) > 0 && body[0] == '<' {
		return nil, fmt.Errorf("HTML response (likely blocked) from %s", url)
	}
	return body, nil
}

func snippet(b []byte) string {
	if len(b) > 120 {
		return string(b[:120])
	}
	return string(b)
}

// FetchPoliticianTrades tries House Stock Watcher (S3), then Senate Stock Watcher.
// 12-month window: Congress has a 45-day disclosure delay.
func FetchPoliticianTrades(ticker string) ([]models.PoliticianTrade, error) {
	cutoff := time.Now().AddDate(-1, 0, 0)
	upperTicker := strings.ToUpper(strings.TrimSpace(ticker))

	var trades []models.PoliticianTrade

	// --- Source 1: House Stock Watcher ---
	houseTrades := fetchHouseTrades(upperTicker, cutoff)
	trades = append(trades, houseTrades...)

	// --- Source 2: Senate Stock Watcher (different bucket, often not blocked) ---
	senateTrades := fetchSenateTrades(upperTicker, cutoff)
	trades = append(trades, senateTrades...)

	// Sort most-recent-first.
	sortByDate(trades)

	if len(trades) > 10 {
		trades = trades[:10]
	}
	if trades == nil {
		trades = []models.PoliticianTrade{}
	}

	log.Printf("[politician] %s → found %d total trades (house+senate)", ticker, len(trades))
	return trades, nil
}

func fetchHouseTrades(ticker string, cutoff time.Time) []models.PoliticianTrade {
	body, err := doGET("https://house-stock-watcher-data.s3-us-west-2.amazonaws.com/data/all_transactions.json")
	if err != nil {
		log.Printf("[politician] house source error: %v", err)
		return nil
	}

	var txns []hswTransaction
	if err := json.Unmarshal(body, &txns); err != nil {
		log.Printf("[politician] house parse error: %v", err)
		return nil
	}

	var out []models.PoliticianTrade
	for _, t := range txns {
		if strings.ToUpper(strings.TrimSpace(t.Ticker)) != ticker {
			continue
		}
		tradeDate, ok := parseHSWDate(t.TransactionDate)
		if !ok || tradeDate.Before(cutoff) {
			continue
		}
		out = append(out, models.PoliticianTrade{
			Name:      shortenName(t.Representative),
			Role:      "House",
			Party:     abbreviateParty(t.Party),
			Action:    strings.ToUpper(t.Type),
			Amount:    t.Amount,
			FiledDate: t.TransactionDate,
		})
	}
	return out
}

func fetchSenateTrades(ticker string, cutoff time.Time) []models.PoliticianTrade {
	body, err := doGET("https://senate-stock-watcher-data.s3-us-west-2.amazonaws.com/aggregate/all_transactions.json")
	if err != nil {
		log.Printf("[politician] senate source error: %v", err)
		return nil
	}

	var txns []sswTransaction
	if err := json.Unmarshal(body, &txns); err != nil {
		log.Printf("[politician] senate parse error: %v", err)
		return nil
	}

	var out []models.PoliticianTrade
	for _, t := range txns {
		if strings.ToUpper(strings.TrimSpace(t.Ticker)) != ticker {
			continue
		}
		tradeDate, ok := parseHSWDate(t.TransactionDate)
		if !ok || tradeDate.Before(cutoff) {
			continue
		}
		out = append(out, models.PoliticianTrade{
			Name:      shortenName(t.Senator),
			Role:      "Senate",
			Party:     abbreviateParty(t.Party),
			Action:    strings.ToUpper(t.Type),
			Amount:    t.Amount,
			FiledDate: t.TransactionDate,
		})
	}
	return out
}

// sortByDate sorts trades most-recent-first using insertion sort.
func sortByDate(trades []models.PoliticianTrade) {
	for i := 1; i < len(trades); i++ {
		for j := i; j > 0; j-- {
			di, _ := parseHSWDate(trades[j].FiledDate)
			dj, _ := parseHSWDate(trades[j-1].FiledDate)
			if di.After(dj) {
				trades[j], trades[j-1] = trades[j-1], trades[j]
			} else {
				break
			}
		}
	}
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
