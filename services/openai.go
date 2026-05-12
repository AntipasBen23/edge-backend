package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"smart-money-backend/models"
)

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type oaiResponseFormat struct {
	Type string `json:"type"`
}

type oaiRequest struct {
	Model          string            `json:"model"`
	Messages       []oaiMessage      `json:"messages"`
	Temperature    float64           `json:"temperature"`
	ResponseFormat oaiResponseFormat `json:"response_format"`
}

type oaiChoice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

type oaiResponse struct {
	Choices []oaiChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// SynthesizeWithOpenAI sends the collected trade data to GPT-4o-mini and returns
// a structured investment synthesis. Retries once on empty response.
func SynthesizeWithOpenAI(ticker, company string, pol []models.PoliticianTrade, ins []models.InsiderTrade) (*models.OpenAISynthesis, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not configured")
	}

	userPrompt := buildPrompt(ticker, company, pol, ins)

	payload := oaiRequest{
		Model: "gpt-4o-mini",
		Messages: []oaiMessage{
			{
				Role: "system",
				Content: "You are an elite quantitative macro analyst at a top hedge fund. " +
					"You synthesize raw financial intelligence data into clear, actionable investment theses. " +
					"Your output must always be structured, precise, and void of generic language. " +
					"Always return a valid JSON object matching the schema provided. " +
					"Never add markdown formatting or code blocks around the JSON.",
			},
			{Role: "user", Content: userPrompt},
		},
		Temperature:    0.3,
		ResponseFormat: oaiResponseFormat{Type: "json_object"},
	}

	result, err := callOpenAI(apiKey, payload)
	if err != nil {
		// Single retry on failure.
		result, err = callOpenAI(apiKey, payload)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func callOpenAI(apiKey string, payload oaiRequest) (*models.OpenAISynthesis, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling openai request: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling openai API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading openai response: %w", err)
	}

	var oaiResp oaiResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return nil, fmt.Errorf("parsing openai response: %w", err)
	}

	if oaiResp.Error != nil {
		return nil, fmt.Errorf("openai error: %s", oaiResp.Error.Message)
	}

	if len(oaiResp.Choices) == 0 || oaiResp.Choices[0].Message.Content == "" {
		return nil, fmt.Errorf("empty response from openai")
	}

	var synthesis models.OpenAISynthesis
	if err := json.Unmarshal([]byte(oaiResp.Choices[0].Message.Content), &synthesis); err != nil {
		return nil, fmt.Errorf("parsing synthesis JSON: %w", err)
	}

	return &synthesis, nil
}

func buildPrompt(ticker, company string, pol []models.PoliticianTrade, ins []models.InsiderTrade) string {
	return fmt.Sprintf(`Act as an elite macro analyst. Below is raw smart-money data gathered from public disclosures for %s - %s.

## POLITICIAN TRADES (Last 30 days):
%s

## INSIDER TRANSACTIONS (Last 30 days):
%s

Synthesize the findings. Identify the strongest signals and contradictions. Flag unusual smart-money activity. Give a clear directional view. Be specific — reference actual names and amounts.

Return ONLY a valid JSON object with these exact fields:
{
  "directional_bias": "BULLISH" or "BEARISH" or "NEUTRAL",
  "time_horizon": "e.g. 3-6 MONTHS",
  "conviction": "HIGH" or "MEDIUM" or "LOW",
  "signal_strength": number between 0 and 10,
  "data_quality": "letter grade e.g. A-, B+",
  "synthesis": "3-4 paragraph analysis referencing specific names and amounts",
  "conviction_score": integer between 0 and 100,
  "suggested_setup": {
    "entry_zone": "price range string",
    "target_1": "price string",
    "target_2": "price string",
    "invalidation": "price string"
  },
  "data_gaps": ["gap 1", "gap 2"]
}`,
		ticker, company,
		formatPolTrades(pol),
		formatInsTrades(ins),
	)
}

func formatPolTrades(trades []models.PoliticianTrade) string {
	if len(trades) == 0 {
		return "No disclosed congressional trades found in last 30 days."
	}
	var sb strings.Builder
	for _, t := range trades {
		sb.WriteString(fmt.Sprintf("- %s (%s, Party:%s): %s — %s filed %s\n",
			t.Name, t.Role, t.Party, t.Action, t.Amount, t.FiledDate))
	}
	return sb.String()
}

func formatInsTrades(trades []models.InsiderTrade) string {
	if len(trades) == 0 {
		return "No SEC Form 4 insider filings found in last 30 days."
	}
	var sb strings.Builder
	for _, t := range trades {
		line := fmt.Sprintf("- %s (%s): %s — %s filed %s", t.Name, t.Role, t.Action, t.Amount, t.FiledDate)
		if t.Note != "" {
			line += fmt.Sprintf(" [%s]", t.Note)
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}
