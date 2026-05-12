package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"smart-money-backend/models"
	"smart-money-backend/services"
)

var tickerPattern = regexp.MustCompile(`^[A-Za-z]{1,6}$`)

func Analyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req models.AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.Ticker = strings.TrimSpace(strings.ToUpper(req.Ticker))
	if !tickerPattern.MatchString(req.Ticker) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ticker: must be 1-6 alphabetic characters"})
		return
	}

	if req.CompanyName == "" {
		req.CompanyName = req.Ticker
	}

	start := time.Now()

	var (
		wg               sync.WaitGroup
		politicianTrades []models.PoliticianTrade
		insiderTrades    []models.InsiderTrade
		polErr           error
		insErr           error
	)

	wg.Add(2)

	go func() {
		defer wg.Done()
		politicianTrades, polErr = services.FetchPoliticianTrades(req.Ticker)
		if polErr != nil {
			log.Printf("[analyze] politician trades error for %s: %v", req.Ticker, polErr)
		}
	}()

	go func() {
		defer wg.Done()
		insiderTrades, insErr = services.FetchInsiderTrades(req.Ticker)
		if insErr != nil {
			log.Printf("[analyze] insider trades error for %s: %v", req.Ticker, insErr)
		}
	}()

	wg.Wait()

	var extraGaps []string
	if polErr != nil {
		extraGaps = append(extraGaps, "Congressional disclosure data temporarily unavailable.")
	}
	if insErr != nil {
		extraGaps = append(extraGaps, "SEC EDGAR Form 4 data temporarily unavailable.")
	}

	synthesis, err := services.SynthesizeWithOpenAI(req.Ticker, req.CompanyName, politicianTrades, insiderTrades)
	if err != nil {
		log.Printf("[analyze] openai error for %s: %v", req.Ticker, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "analysis service temporarily unavailable"})
		return
	}

	// Ensure nil slices marshal as [] not null in JSON.
	if politicianTrades == nil {
		politicianTrades = []models.PoliticianTrade{}
	}
	if insiderTrades == nil {
		insiderTrades = []models.InsiderTrade{}
	}
	if synthesis.DataGaps == nil {
		synthesis.DataGaps = []string{}
	}

	allGaps := append(extraGaps, synthesis.DataGaps...)
	if allGaps == nil {
		allGaps = []string{}
	}

	prefix := req.Ticker
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	reportID := fmt.Sprintf("SMT-%s-%04d", prefix, rand.Intn(9000)+1000)

	report := models.Report{
		Ticker:          req.Ticker,
		CompanyName:     req.CompanyName,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		RunTimeSeconds:  time.Since(start).Seconds(),
		ReportID:        reportID,
		Summary: models.ReportSummary{
			DirectionalBias: synthesis.DirectionalBias,
			TimeHorizon:     synthesis.TimeHorizon,
			Conviction:      synthesis.Conviction,
			SignalStrength:  synthesis.SignalStrength,
			DataQuality:     synthesis.DataQuality,
		},
		PoliticianTrades: politicianTrades,
		InsiderTrades:    insiderTrades,
		Signals:          buildSignals(synthesis),
		Synthesis:        synthesis.Synthesis,
		ConvictionScore:  synthesis.ConvictionScore,
		SuggestedSetup:   synthesis.SuggestedSetup,
		DataGaps:         allGaps,
	}

	writeJSON(w, http.StatusOK, report)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func buildSignals(s *models.OpenAISynthesis) []models.Signal {
	bias := s.DirectionalBias
	if bias == "" {
		bias = "NEUTRAL"
	}

	dataSignal := "WATCH"
	if strings.HasPrefix(s.DataQuality, "A") {
		dataSignal = "BULLISH"
	} else if strings.HasPrefix(s.DataQuality, "C") || strings.HasPrefix(s.DataQuality, "D") {
		dataSignal = "BEARISH"
	}

	return []models.Signal{
		{
			Label:     "Smart Money Confluence",
			Sentiment: bias,
			Summary:   fmt.Sprintf("Congressional and insider signal synthesis complete. Conviction level: %s.", s.Conviction),
		},
		{
			Label:     "Data Quality Assessment",
			Sentiment: dataSignal,
			Summary:   fmt.Sprintf("Data quality graded %s. %d gap(s) flagged for review.", s.DataQuality, len(s.DataGaps)),
		},
		{
			Label:     "Signal Strength",
			Sentiment: strengthToSentiment(s.SignalStrength),
			Summary:   fmt.Sprintf("Composite signal score: %.1f/10. Conviction score: %d/100.", s.SignalStrength, s.ConvictionScore),
		},
	}
}

func strengthToSentiment(strength float64) string {
	switch {
	case strength >= 7:
		return "BULLISH"
	case strength >= 4:
		return "WATCH"
	default:
		return "BEARISH"
	}
}
