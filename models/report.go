package models

type Signal struct {
	Label     string `json:"label"`
	Sentiment string `json:"sentiment"`
	Summary   string `json:"summary"`
}

type SuggestedSetup struct {
	EntryZone    string `json:"entry_zone"`
	Target1      string `json:"target_1"`
	Target2      string `json:"target_2"`
	Invalidation string `json:"invalidation"`
}

type ReportSummary struct {
	DirectionalBias string  `json:"directional_bias"`
	TimeHorizon     string  `json:"time_horizon"`
	Conviction      string  `json:"conviction"`
	SignalStrength  float64 `json:"signal_strength"`
	DataQuality     string  `json:"data_quality"`
}

type Report struct {
	Ticker           string           `json:"ticker"`
	CompanyName      string           `json:"company_name"`
	GeneratedAt      string           `json:"generated_at"`
	RunTimeSeconds   float64          `json:"run_time_seconds"`
	ReportID         string           `json:"report_id"`
	Summary          ReportSummary    `json:"summary"`
	PoliticianTrades []PoliticianTrade `json:"politician_trades"`
	InsiderTrades    []InsiderTrade   `json:"insider_trades"`
	Signals          []Signal         `json:"signals"`
	Synthesis        string           `json:"synthesis"`
	ConvictionScore  int              `json:"conviction_score"`
	SuggestedSetup   SuggestedSetup   `json:"suggested_setup"`
	DataGaps         []string         `json:"data_gaps"`
}

type AnalyzeRequest struct {
	Ticker      string `json:"ticker"`
	CompanyName string `json:"company_name"`
}

type OpenAISynthesis struct {
	DirectionalBias string         `json:"directional_bias"`
	TimeHorizon     string         `json:"time_horizon"`
	Conviction      string         `json:"conviction"`
	SignalStrength  float64        `json:"signal_strength"`
	DataQuality     string         `json:"data_quality"`
	Synthesis       string         `json:"synthesis"`
	ConvictionScore int            `json:"conviction_score"`
	SuggestedSetup  SuggestedSetup `json:"suggested_setup"`
	DataGaps        []string       `json:"data_gaps"`
}
